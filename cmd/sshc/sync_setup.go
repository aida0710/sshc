package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"sshc/internal/api"
	"sshc/internal/remotesync"
)

const (
	maxSyncSetupLine       = 4 << 10
	maxSyncEndpointBytes   = 2048
	maxSyncBucketBytes     = 255
	maxSyncPathBytes       = 255
	maxSyncRegionBytes     = 64
	maxSyncAccessKeyBytes  = 512
	maxSyncSecretKeyBytes  = 512
	maxSyncSnapshotKeySize = 1024
)

var (
	errSyncSetupTTY        = errors.New("sync setup requires an interactive terminal")
	errSyncSetupInput      = errors.New("sync setup input is invalid")
	errSyncSetupIncomplete = errors.New("sync setup target is incomplete")
)

type syncSetupInput struct {
	endpoint         string
	bucket           string
	path             string
	region           string
	direction        api.SyncDirection
	accessKey        []byte
	secretKey        []byte
	reuseCredentials bool
}

func requireSyncSetupTerminal(
	stdin *os.File, prompt io.Writer, terminal passwordTerminal,
) (*os.File, error) {
	promptFile, ok := prompt.(*os.File)
	if !ok || stdin == nil || terminal == nil ||
		!terminal.IsTerminal(int(stdin.Fd())) || !terminal.IsTerminal(int(promptFile.Fd())) {
		return nil, errSyncSetupTTY
	}
	return promptFile, nil
}

func runSyncSetup(
	ctx context.Context,
	engine *engineAPI,
	stdin *os.File,
	stdout io.Writer,
	prompt *os.File,
	terminal passwordTerminal,
) error {
	var current api.SyncStatus
	if err := engine.getJSON(ctx, "/api/v1/sync", &current); err != nil {
		return err
	}
	input, err := readSyncSetupInput(ctx, stdin, prompt, terminal, current)
	if input != nil {
		defer zeroBytes(input.accessKey)
		defer zeroBytes(input.secretKey)
	}
	if err != nil {
		return err
	}

	checkPayload, err := buildSyncSetupCheckPayload(*input)
	if err != nil {
		return errSyncSetupInput
	}
	var checked api.SyncSetupCheckResponse
	if err := engine.sendSecretJSON(ctx, http.MethodPost, "/api/v1/sync/setup/check", checkPayload, &checked); err != nil {
		return err
	}
	if !checked.State.Valid() || checked.CheckedAt == "" {
		return errEngineInvalidResponse
	}
	if checked.State == api.Incomplete {
		return errSyncSetupIncomplete
	}
	if checked.State == api.Existing && (checked.Etag == nil || *checked.Etag == "") {
		return errEngineInvalidResponse
	}

	var syncKey []byte
	reuseKey := false
	if checked.State == api.Existing {
		label := "Sync key: "
		if current.KeyConfigured {
			label = "Sync key [configured; Enter to keep]: "
		}
		syncKey, err = promptMaskedSetupValue(ctx, stdin, prompt, terminal, label)
		defer zeroBytes(syncKey)
		if err != nil {
			return err
		}
		syncKey = bytes.TrimSpace(syncKey)
		reuseKey = len(syncKey) == 0 && current.KeyConfigured
		if (!reuseKey && len(syncKey) == 0) || len(syncKey) > maxSyncSnapshotKeySize || !utf8.Valid(syncKey) {
			return errSyncSetupInput
		}
	}

	completePayload, err := buildSyncSetupCompletePayload(*input, checked, syncKey, reuseKey)
	if err != nil {
		return errSyncSetupInput
	}
	var completed api.SyncSetupResponse
	if err := engine.sendSecretJSON(ctx, http.MethodPut, "/api/v1/sync/setup", completePayload, &completed); err != nil {
		return err
	}
	if !completed.Status.Configured || !completed.Status.KeyConfigured {
		return errEngineInvalidResponse
	}
	if checked.State == api.Empty {
		if completed.GeneratedKey == nil || strings.TrimSpace(*completed.GeneratedKey) == "" {
			return errEngineInvalidResponse
		}
		if !safeGeneratedSyncKey(*completed.GeneratedKey) {
			return errEngineInvalidResponse
		}
		if _, err := fmt.Fprintf(prompt,
			"Generated sync key (save it now; it will not be shown again): %s\n",
			*completed.GeneratedKey); err != nil {
			return err
		}
	} else if completed.GeneratedKey != nil {
		return errEngineInvalidResponse
	}
	writeSyncStatus(stdout, completed.Status)
	return nil
}

func readSyncSetupInput(
	ctx context.Context, stdin *os.File, prompt *os.File, terminal passwordTerminal, current api.SyncStatus,
) (*syncSetupInput, error) {
	endpointDefault := current.Endpoint
	if endpointDefault == "" {
		endpointDefault = "https://"
	}
	endpoint, err := promptVisibleSetup(ctx, stdin, prompt,
		setupVisibleLabel("Endpoint", endpointDefault), endpointDefault)
	if err != nil {
		return nil, err
	}
	bucket, err := promptVisibleSetup(ctx, stdin, prompt,
		setupVisibleLabel("Bucket", current.Bucket), current.Bucket)
	if err != nil {
		return nil, err
	}
	pathDefault := ""
	if current.Path != nil {
		pathDefault = *current.Path
	}
	path, err := promptVisibleSetup(ctx, stdin, prompt,
		setupVisibleLabel("Path", pathDefault), pathDefault)
	if err != nil {
		return nil, err
	}
	regionDefault := "auto"
	if current.Region != nil && *current.Region != "" {
		regionDefault = *current.Region
	}
	region, err := promptVisibleSetup(ctx, stdin, prompt,
		setupVisibleLabel("Region", regionDefault), regionDefault)
	if err != nil {
		return nil, err
	}
	directionDefault := string(current.Direction)
	if _, ok := remotesync.ParseDirection(directionDefault); !ok {
		directionDefault = "both"
	}
	directionName, err := promptVisibleSetup(ctx, stdin, prompt,
		fmt.Sprintf("Direction [%s] (both/push/pull): ", safeTerminalCell(directionDefault)), directionDefault)
	if err != nil {
		return nil, err
	}
	direction, ok := remotesync.ParseDirection(directionName)
	if !ok || !validSyncSetupTarget(endpoint, bucket, path, region) {
		return nil, errSyncSetupInput
	}

	credentialConfigured := current.Configured
	accessLabel := "Access key ID: "
	secretLabel := "Secret access key: "
	if credentialConfigured {
		accessLabel = "Access key ID [configured; Enter to keep]: "
		if current.AccessKeySuffix != nil && *current.AccessKeySuffix != "" {
			accessLabel = fmt.Sprintf("Access key ID [%s; Enter to keep]: ",
				maskedAccessKeySuffix(*current.AccessKeySuffix))
		}
		secretLabel = "Secret access key [configured; Enter to keep]: "
	}
	accessKey, err := promptMaskedSetupValue(ctx, stdin, prompt, terminal, accessLabel)
	if err != nil {
		zeroBytes(accessKey)
		return nil, err
	}
	secretKey, err := promptMaskedSetupValue(ctx, stdin, prompt, terminal, secretLabel)
	if err != nil {
		zeroBytes(accessKey)
		zeroBytes(secretKey)
		return nil, err
	}
	reuseCredentials := credentialConfigured && len(accessKey) == 0 && len(secretKey) == 0
	if (!reuseCredentials && (len(accessKey) == 0 || len(secretKey) == 0)) ||
		len(accessKey) > maxSyncAccessKeyBytes || !utf8.Valid(accessKey) ||
		len(secretKey) > maxSyncSecretKeyBytes || !utf8.Valid(secretKey) {
		zeroBytes(accessKey)
		zeroBytes(secretKey)
		return nil, errSyncSetupInput
	}
	return &syncSetupInput{
		endpoint: endpoint, bucket: bucket, path: path, region: region,
		direction: api.SyncDirection(direction), accessKey: accessKey, secretKey: secretKey,
		reuseCredentials: reuseCredentials,
	}, nil
}

func setupVisibleLabel(name, value string) string {
	return fmt.Sprintf("%s [%s]: ", name, safeTerminalCell(value))
}

func maskedAccessKeySuffix(suffix string) string {
	characters := []rune(suffix)
	if len(characters) > 5 {
		characters = characters[len(characters)-5:]
	}
	return "*****" + safeTerminalCell(string(characters))
}

// promptMaskedSetupValue confirms hidden input with stars after Enter. The
// actual value is never written to a terminal or retained in a string.
func promptMaskedSetupValue(
	ctx context.Context, stdin *os.File, prompt io.Writer, terminal passwordTerminal, label string,
) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	prompted := false
	maskCount := 0
	promptInput := func() error {
		if _, err := fmt.Fprint(prompt, label); err != nil {
			return err
		}
		prompted = true
		return nil
	}
	feedback := func(next int) error {
		if next > maskCount {
			if _, err := fmt.Fprint(prompt, strings.Repeat("*", next-maskCount)); err != nil {
				return err
			}
		} else if next < maskCount {
			if _, err := fmt.Fprint(prompt, strings.Repeat("\b \b", maskCount-next)); err != nil {
				return err
			}
		}
		maskCount = next
		return nil
	}
	live, liveFeedback := terminal.(maskedPasswordTerminal)
	var typed []byte
	var err error
	if liveFeedback {
		typed, err = live.ReadPasswordMasked(ctx, stdin, promptInput, feedback)
	} else {
		typed, err = terminal.ReadPassword(ctx, stdin, promptInput)
	}
	var outputErr error
	if prompted {
		if err == nil && !liveFeedback && len(typed) != 0 {
			_, outputErr = fmt.Fprint(prompt, strings.Repeat("*", utf8.RuneCount(typed)))
		}
		if _, newlineErr := fmt.Fprintln(prompt); outputErr == nil {
			outputErr = newlineErr
		}
	}
	if err != nil {
		return typed, err
	}
	return typed, outputErr
}

func promptVisibleSetup(
	ctx context.Context, input *os.File, prompt *os.File, label, defaultValue string,
) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if _, err := fmt.Fprint(prompt, label); err != nil {
		return "", err
	}
	line, err := readBoundedVisibleLine(ctx, input)
	defer zeroBytes(line)
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(line))
	if value == "" {
		value = defaultValue
	}
	return value, nil
}

// readBoundedVisibleLine reads exactly through one newline without buffering
// bytes from the next hidden prompt. The fixed capacity also prevents an old
// heap buffer from retaining abandoned input after append growth.
func readBoundedVisibleLine(ctx context.Context, input *os.File) ([]byte, error) {
	line := make([]byte, 0, maxSyncSetupLine)
	for {
		if err := ctx.Err(); err != nil {
			zeroBytes(line)
			return nil, err
		}
		var one [1]byte
		count, err := input.Read(one[:])
		if count > 0 {
			switch one[0] {
			case '\n':
				if !utf8.Valid(line) {
					zeroBytes(line)
					return nil, errSyncSetupInput
				}
				return line, nil
			case '\r':
				// Canonical Unix terminals normally deliver Enter as LF, while
				// Windows consoles can deliver CRLF. Ignore CR so its following
				// LF is consumed by this prompt instead of the next one.
			case 0x03:
				zeroBytes(line)
				return nil, context.Canceled
			default:
				if len(line) == maxSyncSetupLine {
					zeroBytes(line)
					return nil, errSyncSetupInput
				}
				line = append(line, one[0])
			}
		}
		if err != nil {
			if ctx.Err() != nil {
				zeroBytes(line)
				return nil, ctx.Err()
			}
			if errors.Is(err, io.EOF) && len(line) > 0 && utf8.Valid(line) {
				return line, nil
			}
			zeroBytes(line)
			return nil, err
		}
		if count == 0 {
			zeroBytes(line)
			return nil, io.ErrNoProgress
		}
	}
}

func validSyncSetupTarget(endpoint, bucket, path, region string) bool {
	if len(endpoint) == 0 || len(endpoint) > maxSyncEndpointBytes ||
		len(bucket) == 0 || len(bucket) > maxSyncBucketBytes ||
		len(path) > maxSyncPathBytes || len(region) == 0 || len(region) > maxSyncRegionBytes {
		return false
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Opaque != "" ||
		(parsed.Path != "" && parsed.Path != "/") || parsed.RawPath != "" || parsed.RawQuery != "" ||
		parsed.ForceQuery || parsed.Fragment != "" {
		return false
	}
	if !safeSyncSetupName(bucket) || !safeSyncSetupPath(strings.Trim(path, "/")) {
		return false
	}
	return true
}

func safeGeneratedSyncKey(key string) bool {
	if len(key) == 0 || len(key) > maxSyncSnapshotKeySize || !utf8.ValidString(key) {
		return false
	}
	for _, character := range key {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func safeSyncSetupPath(path string) bool {
	if path == "" {
		return true
	}
	if strings.Contains(path, "..") {
		return false
	}
	for _, segment := range strings.Split(path, "/") {
		if !safeSyncSetupName(segment) {
			return false
		}
	}
	return true
}

func safeSyncSetupName(name string) bool {
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "..") {
		return false
	}
	for _, character := range name {
		switch {
		case character >= 'a' && character <= 'z',
			character >= 'A' && character <= 'Z',
			character >= '0' && character <= '9',
			character == '-', character == '.', character == '_':
		default:
			return false
		}
	}
	return filepath.Base(name) == name
}

type zeroJSONField struct {
	name    string
	value   []byte
	boolean *bool
}

func buildSyncSetupCheckPayload(input syncSetupInput) ([]byte, error) {
	reuse := input.reuseCredentials
	fields := []zeroJSONField{
		{name: "endpoint", value: []byte(input.endpoint)},
		{name: "bucket", value: []byte(input.bucket)},
		{name: "path", value: []byte(input.path)},
		{name: "region", value: []byte(input.region)},
		{name: "reuseCredentials", boolean: &reuse},
	}
	if !input.reuseCredentials {
		fields = append(fields,
			zeroJSONField{name: "accessKeyId", value: input.accessKey},
			zeroJSONField{name: "secretAccessKey", value: input.secretKey},
		)
	}
	return buildZeroJSON(fields)
}

func buildSyncSetupCompletePayload(
	input syncSetupInput, checked api.SyncSetupCheckResponse, syncKey []byte, reuseKey bool,
) ([]byte, error) {
	history := checked.HistoryPresent
	reuseCredentials := input.reuseCredentials
	fields := []zeroJSONField{
		{name: "endpoint", value: []byte(input.endpoint)},
		{name: "bucket", value: []byte(input.bucket)},
		{name: "path", value: []byte(input.path)},
		{name: "region", value: []byte(input.region)},
		{name: "direction", value: []byte(input.direction)},
		{name: "reuseCredentials", boolean: &reuseCredentials},
		{name: "expectedState", value: []byte(checked.State)},
		{name: "historyPresent", boolean: &history},
		{name: "reuseKey", boolean: &reuseKey},
	}
	if !input.reuseCredentials {
		fields = append(fields,
			zeroJSONField{name: "accessKeyId", value: input.accessKey},
			zeroJSONField{name: "secretAccessKey", value: input.secretKey},
		)
	}
	if checked.Etag != nil {
		fields = append(fields, zeroJSONField{name: "expectedETag", value: []byte(*checked.Etag)})
	}
	fields = append(fields, zeroJSONField{name: "key", value: syncKey})
	return buildZeroJSON(fields)
}

func buildZeroJSON(fields []zeroJSONField) ([]byte, error) {
	total := 2
	for index, field := range fields {
		nameSize, err := vaultJSONStringSize([]byte(field.name))
		if err != nil {
			return nil, err
		}
		valueSize := 5
		if field.boolean == nil {
			valueSize, err = vaultJSONStringSize(field.value)
			if err != nil {
				return nil, err
			}
		} else if *field.boolean {
			valueSize = 4
		}
		total += nameSize + 1 + valueSize
		if index != 0 {
			total++
		}
	}
	payload := make([]byte, 0, total)
	payload = append(payload, '{')
	for index, field := range fields {
		if index != 0 {
			payload = append(payload, ',')
		}
		var err error
		payload, err = appendVaultJSONString(payload, []byte(field.name))
		if err != nil {
			zeroBytes(payload)
			return nil, err
		}
		payload = append(payload, ':')
		if field.boolean != nil {
			if *field.boolean {
				payload = append(payload, "true"...)
			} else {
				payload = append(payload, "false"...)
			}
			continue
		}
		payload, err = appendVaultJSONString(payload, field.value)
		if err != nil {
			zeroBytes(payload)
			return nil, err
		}
	}
	payload = append(payload, '}')
	if len(payload) != cap(payload) {
		zeroBytes(payload)
		return nil, errSyncSetupInput
	}
	return payload, nil
}
