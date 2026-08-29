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
	endpoint  string
	bucket    string
	path      string
	region    string
	direction api.SyncDirection
	accessKey []byte
	secretKey []byte
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
	input, err := readSyncSetupInput(ctx, stdin, prompt, terminal)
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
	if checked.State == api.Existing {
		syncKey, err = promptVaultPassword(ctx, stdin, prompt, terminal, "Sync key: ")
		defer zeroBytes(syncKey)
		if err != nil {
			return err
		}
		syncKey = bytes.TrimSpace(syncKey)
		if len(syncKey) == 0 || len(syncKey) > maxSyncSnapshotKeySize || !utf8.Valid(syncKey) {
			return errSyncSetupInput
		}
	}

	completePayload, err := buildSyncSetupCompletePayload(*input, checked, syncKey)
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
	ctx context.Context, stdin *os.File, prompt *os.File, terminal passwordTerminal,
) (*syncSetupInput, error) {
	endpoint, err := promptVisibleSetup(ctx, stdin, prompt, "Endpoint [https://]: ", "https://")
	if err != nil {
		return nil, err
	}
	bucket, err := promptVisibleSetup(ctx, stdin, prompt, "Bucket: ", "")
	if err != nil {
		return nil, err
	}
	path, err := promptVisibleSetup(ctx, stdin, prompt, "Path []: ", "")
	if err != nil {
		return nil, err
	}
	region, err := promptVisibleSetup(ctx, stdin, prompt, "Region [auto]: ", "auto")
	if err != nil {
		return nil, err
	}
	directionName, err := promptVisibleSetup(ctx, stdin, prompt, "Direction [both]: ", "both")
	if err != nil {
		return nil, err
	}
	direction, ok := remotesync.ParseDirection(directionName)
	if !ok || !validSyncSetupTarget(endpoint, bucket, path, region) {
		return nil, errSyncSetupInput
	}

	accessKey, err := promptVaultPassword(ctx, stdin, prompt, terminal, "Access key ID: ")
	if err != nil {
		zeroBytes(accessKey)
		return nil, err
	}
	secretKey, err := promptVaultPassword(ctx, stdin, prompt, terminal, "Secret access key: ")
	if err != nil {
		zeroBytes(accessKey)
		zeroBytes(secretKey)
		return nil, err
	}
	if len(accessKey) == 0 || len(accessKey) > maxSyncAccessKeyBytes || !utf8.Valid(accessKey) ||
		len(secretKey) == 0 || len(secretKey) > maxSyncSecretKeyBytes || !utf8.Valid(secretKey) {
		zeroBytes(accessKey)
		zeroBytes(secretKey)
		return nil, errSyncSetupInput
	}
	return &syncSetupInput{
		endpoint: endpoint, bucket: bucket, path: path, region: region,
		direction: api.SyncDirection(direction), accessKey: accessKey, secretKey: secretKey,
	}, nil
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
	return buildZeroJSON([]zeroJSONField{
		{name: "endpoint", value: []byte(input.endpoint)},
		{name: "bucket", value: []byte(input.bucket)},
		{name: "path", value: []byte(input.path)},
		{name: "region", value: []byte(input.region)},
		{name: "accessKeyId", value: input.accessKey},
		{name: "secretAccessKey", value: input.secretKey},
	})
}

func buildSyncSetupCompletePayload(
	input syncSetupInput, checked api.SyncSetupCheckResponse, syncKey []byte,
) ([]byte, error) {
	history := checked.HistoryPresent
	fields := []zeroJSONField{
		{name: "endpoint", value: []byte(input.endpoint)},
		{name: "bucket", value: []byte(input.bucket)},
		{name: "path", value: []byte(input.path)},
		{name: "region", value: []byte(input.region)},
		{name: "direction", value: []byte(input.direction)},
		{name: "accessKeyId", value: input.accessKey},
		{name: "secretAccessKey", value: input.secretKey},
		{name: "expectedState", value: []byte(checked.State)},
		{name: "historyPresent", boolean: &history},
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
