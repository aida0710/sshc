package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
	"unicode/utf8"

	"golang.org/x/term"

	"sshc/internal/handoff"
	"sshc/internal/httpserver"
)

const (
	maxVaultResponseBody  = 64 << 10
	maxVaultRequestBody   = 4 << 10
	maxVaultPasswordBytes = 4 << 10
	vaultCommandTimeout   = 3 * time.Minute
)

var (
	errVaultResponseTooLarge = errors.New("vault response is too large")
	errVaultRequestTooLarge  = errors.New("vault request is too large")
	errVaultPasswordTooLong  = errors.New("vault password is too long")
	errInvalidVaultResponse  = errors.New("invalid vault response")
	errInvalidPasswordText   = errors.New("password is not valid UTF-8")
)

// passwordTerminal は、マスターパスワードを通常の stdin pipe から分離する。
// no-echo 読み取りができない入力を先に拒むことで、パイプや履歴へ秘密を置かない。
type passwordTerminal interface {
	IsTerminal(fd int) bool
	// ReadPassword calls prompt only after no-echo input is active. It restores the
	// terminal mode before returning, including when prompt or the read fails.
	ReadPassword(ctx context.Context, input *os.File, prompt func() error) ([]byte, error)
}

// maskedPasswordTerminal is implemented by real OS terminals so setup can
// confirm each hidden character as it is typed. Test doubles and alternative
// terminals may omit it; the caller then confirms the input after Enter.
type maskedPasswordTerminal interface {
	ReadPasswordMasked(
		ctx context.Context, input *os.File, prompt func() error, feedback func(int) error,
	) ([]byte, error)
}

type systemPasswordTerminal struct{}

func (systemPasswordTerminal) IsTerminal(fd int) bool { return term.IsTerminal(fd) }

// runVault は、起動済み engine の Vault operation だけを行う。engine を起動しない
// のは、desktop と headless の owner をこの補助コマンドが勝手に選ばないためである。
func runVault(
	ctx context.Context,
	action string,
	stateDir string,
	client *http.Client,
	stdin *os.File,
	stdout, stderr io.Writer,
	terminal passwordTerminal,
) int {
	if err := ctx.Err(); err != nil {
		return 130
	}
	if action != "status" && action != "create" && action != "unlock" &&
		action != "lock" && action != "change-password" {
		fmt.Fprintln(stderr, "sshc: unknown vault action")
		return 2
	}

	needsPassword := action == "create" || action == "unlock" || action == "change-password"
	if needsPassword && (stdin == nil || terminal == nil || !terminal.IsTerminal(int(stdin.Fd()))) {
		fmt.Fprintln(stderr, "sshc: vault passwords require an interactive terminal")
		return 1
	}

	found, err := readHandoff(stateDir)
	if err != nil {
		fmt.Fprintln(stderr, "sshc: no compatible running engine; start the desktop app or run sshc engine")
		return 1
	}
	status, err := fetchVaultStatus(ctx, found, client)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return 130
		}
		fmt.Fprintln(stderr, "sshc: the running engine did not return a valid vault status")
		return 1
	}
	if ctx.Err() != nil {
		return 130
	}
	if action == "status" {
		// `sshc status` と同じ表を出す。同じ内容を二通りに書いていた間、
		// 片方に項目を足しても、もう片方は古いままになった。
		writeStatus(stdout, found, status)
		return 0
	}

	switch action {
	case "create":
		if status.Vault {
			fmt.Fprintln(stderr, "sshc: a vault already exists")
			return 1
		}
		return runVaultCreate(ctx, found, client, stdin, stdout, stderr, terminal)
	case "unlock":
		if !status.Vault {
			fmt.Fprintln(stderr, "sshc: no vault exists; run sshc vault create")
			return 1
		}
		if status.Unlocked {
			fmt.Fprintln(stdout, "vault is already unlocked")
			return 0
		}
		return runVaultUnlock(ctx, found, client, stdin, stdout, stderr, terminal)
	case "lock":
		return runVaultLock(ctx, found, client, stdout, stderr)
	case "change-password":
		if !status.Vault {
			fmt.Fprintln(stderr, "sshc: no vault exists; run sshc vault create")
			return 1
		}
		if !status.Unlocked {
			fmt.Fprintln(stderr, "sshc: the vault is locked; run sshc vault unlock first")
			return 1
		}
		return runVaultChange(ctx, found, client, stdin, stdout, stderr, terminal)
	default:
		return 2
	}
}

func runVaultCreate(
	ctx context.Context, found handoff.Handoff, client *http.Client, stdin *os.File,
	stdout, stderr io.Writer, terminal passwordTerminal,
) int {
	next, err := promptVaultPassword(ctx, stdin, stderr, terminal, "New master password: ")
	defer zeroBytes(next)
	if err != nil {
		return vaultPromptFailure(ctx, err, stderr)
	}
	if ctx.Err() != nil {
		return 130
	}
	confirmation, err := promptVaultPassword(ctx, stdin, stderr, terminal, "Confirm new master password: ")
	defer zeroBytes(confirmation)
	if err != nil {
		return vaultPromptFailure(ctx, err, stderr)
	}
	if ctx.Err() != nil {
		return 130
	}
	if !bytes.Equal(next, confirmation) {
		fmt.Fprintln(stderr, "sshc: password confirmation did not match")
		return 1
	}
	payload, err := vaultPassphrasePayload(next)
	if err != nil {
		fmt.Fprintln(stderr, "sshc: the password could not be encoded safely")
		return 1
	}
	zeroBytes(next)
	zeroBytes(confirmation)
	return finishVaultMutation(ctx, client, found, httpserver.VaultCreatePath, payload, "vault created and unlocked", stderr, stdout)
}

func runVaultUnlock(
	ctx context.Context, found handoff.Handoff, client *http.Client, stdin *os.File,
	stdout, stderr io.Writer, terminal passwordTerminal,
) int {
	password, err := promptVaultPassword(ctx, stdin, stderr, terminal, "Master password: ")
	defer zeroBytes(password)
	if err != nil {
		return vaultPromptFailure(ctx, err, stderr)
	}
	if ctx.Err() != nil {
		return 130
	}
	payload, err := vaultPassphrasePayload(password)
	if err != nil {
		fmt.Fprintln(stderr, "sshc: the password could not be encoded safely")
		return 1
	}
	zeroBytes(password)
	return finishVaultMutation(ctx, client, found, httpserver.VaultUnlockPath, payload, "vault unlocked", stderr, stdout)
}

func runVaultLock(
	ctx context.Context, found handoff.Handoff, client *http.Client, stdout, stderr io.Writer,
) int {
	return finishVaultMutation(ctx, client, found, httpserver.VaultLockPath, []byte("{}"), "vault locked", stderr, stdout)
}

func runVaultChange(
	ctx context.Context, found handoff.Handoff, client *http.Client, stdin *os.File,
	stdout, stderr io.Writer, terminal passwordTerminal,
) int {
	current, err := promptVaultPassword(ctx, stdin, stderr, terminal, "Current master password: ")
	defer zeroBytes(current)
	if err != nil {
		return vaultPromptFailure(ctx, err, stderr)
	}
	if ctx.Err() != nil {
		return 130
	}
	next, err := promptVaultPassword(ctx, stdin, stderr, terminal, "New master password: ")
	defer zeroBytes(next)
	if err != nil {
		return vaultPromptFailure(ctx, err, stderr)
	}
	if ctx.Err() != nil {
		return 130
	}
	confirmation, err := promptVaultPassword(ctx, stdin, stderr, terminal, "Confirm new master password: ")
	defer zeroBytes(confirmation)
	if err != nil {
		return vaultPromptFailure(ctx, err, stderr)
	}
	if ctx.Err() != nil {
		return 130
	}
	if !bytes.Equal(next, confirmation) {
		fmt.Fprintln(stderr, "sshc: password confirmation did not match")
		return 1
	}
	payload, err := vaultChangePayload(current, next)
	if err != nil {
		fmt.Fprintln(stderr, "sshc: a password could not be encoded safely")
		return 1
	}
	zeroBytes(current)
	zeroBytes(next)
	zeroBytes(confirmation)
	return finishVaultMutation(ctx, client, found, httpserver.VaultChangePath, payload, "vault password changed", stderr, stdout)
}

func promptVaultPassword(
	ctx context.Context, stdin *os.File, stderr io.Writer, terminal passwordTerminal, prompt string,
) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	prompted := false
	typed, err := terminal.ReadPassword(ctx, stdin, func() error {
		if _, err := fmt.Fprint(stderr, prompt); err != nil {
			return err
		}
		prompted = true
		return nil
	})
	var newlineErr error
	if prompted {
		_, newlineErr = fmt.Fprintln(stderr)
	}
	if err != nil {
		return typed, err
	}
	return typed, newlineErr
}

func vaultPromptFailure(ctx context.Context, err error, stderr io.Writer) int {
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return 130
	}
	fmt.Fprintln(stderr, "sshc: could not read the vault password")
	return 1
}

func finishVaultMutation(
	ctx context.Context,
	client *http.Client,
	found handoff.Handoff,
	path string,
	payload []byte,
	success string,
	stderr, stdout io.Writer,
) int {
	if err := ctx.Err(); err != nil {
		zeroBytes(payload)
		return 130
	}
	response, err := sendVaultPOST(ctx, client, found, path, payload)
	if err != nil {
		writeUncertainVaultResult(path, stderr)
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return 130
		}
		return 1
	}
	body, err := readAndCloseVaultResponse(response)
	defer zeroBytes(body)
	if err != nil {
		fmt.Fprintln(stderr, "sshc: the running engine returned an invalid vault response")
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return 130
		}
		return 1
	}
	if response.StatusCode == http.StatusNoContent {
		fmt.Fprintln(stdout, success)
		return 0
	}
	switch response.StatusCode {
	case http.StatusUnauthorized:
		if path == httpserver.VaultUnlockPath || path == httpserver.VaultChangePath {
			fmt.Fprintln(stderr, "sshc: the vault password or engine authentication was refused")
		} else {
			fmt.Fprintln(stderr, "sshc: engine authentication was refused")
		}
	case http.StatusConflict:
		fmt.Fprintln(stderr, "sshc: the vault state changed; run sshc vault status and try again")
	case http.StatusBadRequest:
		fmt.Fprintln(stderr, "sshc: the vault password or request was not accepted")
	case http.StatusRequestEntityTooLarge:
		fmt.Fprintln(stderr, "sshc: the vault password or request is too large")
	default:
		fmt.Fprintln(stderr, "sshc: the vault operation failed")
	}
	return 1
}

func writeUncertainVaultResult(path string, stderr io.Writer) {
	if path == httpserver.VaultChangePath {
		fmt.Fprintln(stderr, "sshc: password change outcome is uncertain; the local password may already have changed. Run sshc vault lock (existing SSH sessions stay connected), then run sshc vault unlock with the new password first and the old password second.")
		return
	}
	fmt.Fprintln(stderr, "sshc: vault request outcome is uncertain; run sshc vault status to check the result")
}

func fetchVaultStatus(
	ctx context.Context, found handoff.Handoff, client *http.Client,
) (statusAnswer, error) {
	request, err := newHandoffRequest(ctx, found, http.MethodGet, httpserver.VaultStatusPath, nil)
	if err != nil {
		return statusAnswer{}, err
	}
	response, err := vaultClient(client).Do(request)
	if err != nil {
		if response != nil {
			discardAndCloseVaultResponse(response)
		}
		return statusAnswer{}, err
	}
	body, err := readAndCloseVaultResponse(response)
	defer zeroBytes(body)
	if err != nil || response.StatusCode != http.StatusOK {
		return statusAnswer{}, errInvalidVaultResponse
	}

	type statusWire struct {
		Owner           handoff.Owner `json:"owner"`
		Version         string        `json:"version"`
		ProtocolVersion int           `json:"protocolVersion"`
		Vault           *bool         `json:"vault"`
		Unlocked        *bool         `json:"unlocked"`
		Sessions        *int          `json:"sessions"`
	}
	var wire statusWire
	if err := decodeVaultResponseJSON(body, &wire); err != nil || wire.Vault == nil || wire.Unlocked == nil || wire.Sessions == nil {
		return statusAnswer{}, errInvalidVaultResponse
	}
	if wire.Owner != found.Owner || wire.Version != found.Version || wire.ProtocolVersion != found.ProtocolVersion ||
		*wire.Sessions < 0 || (!*wire.Vault && *wire.Unlocked) {
		return statusAnswer{}, errInvalidVaultResponse
	}
	return statusAnswer{
		Owner: wire.Owner, Version: wire.Version, ProtocolVersion: wire.ProtocolVersion,
		Vault: *wire.Vault, Unlocked: *wire.Unlocked, Sessions: *wire.Sessions,
	}, nil
}

// sendVaultPOST は payload の所有権を受け取り、Do が戻るすべての経路で消去する。
// oneShotSecretPayload は Seek/GetBody を持たず、redirect に秘密を再送できない。
func sendVaultPOST(
	ctx context.Context,
	client *http.Client,
	found handoff.Handoff,
	path string,
	payload []byte,
) (*http.Response, error) {
	defer zeroBytes(payload)
	request, err := newHandoffRequest(ctx, found, http.MethodPost, path, &oneShotSecretPayload{body: payload})
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := vaultClient(client).Do(request)
	if err != nil && response != nil {
		discardAndCloseVaultResponse(response)
	}
	return response, err
}

func vaultClient(client *http.Client) *http.Client {
	return noRedirectClient(client)
}

// vaultCommandClient は対話的な Vault 操作を短い接続確認タイムアウトから分離する。
// パスワード変更では最長 1 分のリモートスナップショット書き込みを 2 回待つ可能性が
// あるが、キャンセルは引き続きリクエストコンテキストで伝播する。
func vaultCommandClient(client *http.Client) *http.Client {
	if client == nil {
		client = http.DefaultClient
	}
	cloned := *client
	cloned.Timeout = vaultCommandTimeout
	return &cloned
}

func readAndCloseVaultResponse(response *http.Response) ([]byte, error) {
	if response == nil || response.Body == nil {
		return nil, errInvalidVaultResponse
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxVaultResponseBody+1))
	if err != nil {
		zeroBytes(body)
		return nil, err
	}
	if len(body) > maxVaultResponseBody {
		zeroBytes(body)
		return nil, errVaultResponseTooLarge
	}
	return body, nil
}

// discardAndCloseVaultResponse は、Do が error と response の両方を返した経路でも
// server が反射した秘密を heap に残さない。close だけでは読み済みbufferは消えない。
func discardAndCloseVaultResponse(response *http.Response) {
	body, _ := readAndCloseVaultResponse(response)
	zeroBytes(body)
}

func decodeVaultResponseJSON(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errInvalidVaultResponse
	}
	return nil
}

func vaultPassphrasePayload(password []byte) ([]byte, error) {
	encodedSize, err := vaultJSONStringSize(password)
	if err != nil {
		return nil, err
	}
	totalSize := len(`{"passphrase":`) + encodedSize + 1
	if totalSize > maxVaultRequestBody {
		return nil, errVaultRequestTooLarge
	}
	// 容量を固定し、JSON エスケープ中の append がパスワードの断片を含むヒープバッファを
	// 放棄しないようにする。
	payload := make([]byte, 0, totalSize)
	payload = append(payload, `{"passphrase":`...)
	payload, err = appendVaultJSONString(payload, password)
	if err != nil {
		zeroBytes(payload)
		return nil, err
	}
	payload = append(payload, '}')
	return payload, nil
}

func vaultChangePayload(current, next []byte) ([]byte, error) {
	currentSize, err := vaultJSONStringSize(current)
	if err != nil {
		return nil, err
	}
	nextSize, err := vaultJSONStringSize(next)
	if err != nil {
		return nil, err
	}
	totalSize := len(`{"current":`) + currentSize + len(`,"next":`) + nextSize + 1
	if totalSize > maxVaultRequestBody {
		return nil, errVaultRequestTooLarge
	}
	payload := make([]byte, 0, totalSize)
	payload = append(payload, `{"current":`...)
	payload, err = appendVaultJSONString(payload, current)
	if err == nil {
		payload = append(payload, `,"next":`...)
		payload, err = appendVaultJSONString(payload, next)
	}
	if err != nil {
		zeroBytes(payload)
		return nil, err
	}
	payload = append(payload, '}')
	return payload, nil
}

func vaultJSONStringSize(password []byte) (int, error) {
	if !utf8.Valid(password) {
		return 0, errInvalidPasswordText
	}
	size := 2 // surrounding quotes
	for offset := 0; offset < len(password); {
		value := password[offset]
		if value >= utf8.RuneSelf {
			runeValue, runeSize := utf8.DecodeRune(password[offset:])
			if runeValue == '\u2028' || runeValue == '\u2029' {
				size += 6
			} else {
				size += runeSize
			}
			offset += runeSize
			continue
		}
		offset++
		switch value {
		case '"', '\\', '\b', '\f', '\n', '\r', '\t':
			size += 2
		default:
			if value < 0x20 {
				size += 6
			} else {
				size++
			}
		}
	}
	return size, nil
}

// appendVaultJSONString は password を string に変えず JSON string を作る。
// 無効な UTF-8 を replacement rune に変えると入力と送信値が異なるため拒否する。
func appendVaultJSONString(destination, password []byte) ([]byte, error) {
	if !utf8.Valid(password) {
		return destination, errInvalidPasswordText
	}
	destination = append(destination, '"')
	for offset := 0; offset < len(password); {
		value := password[offset]
		if value >= utf8.RuneSelf {
			runeValue, size := utf8.DecodeRune(password[offset:])
			if runeValue == '\u2028' {
				destination = append(destination, `\u2028`...)
			} else if runeValue == '\u2029' {
				destination = append(destination, `\u2029`...)
			} else {
				destination = append(destination, password[offset:offset+size]...)
			}
			offset += size
			continue
		}
		offset++
		switch value {
		case '"', '\\':
			destination = append(destination, '\\', value)
		case '\b':
			destination = append(destination, '\\', 'b')
		case '\f':
			destination = append(destination, '\\', 'f')
		case '\n':
			destination = append(destination, '\\', 'n')
		case '\r':
			destination = append(destination, '\\', 'r')
		case '\t':
			destination = append(destination, '\\', 't')
		default:
			if value < 0x20 {
				const hex = "0123456789abcdef"
				destination = append(destination, '\\', 'u', '0', '0', hex[value>>4], hex[value&0x0f])
			} else {
				destination = append(destination, value)
			}
		}
	}
	destination = append(destination, '"')
	return destination, nil
}

func zeroBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
