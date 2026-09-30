package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/term"

	"sshc/internal/handoff"
	"sshc/internal/httpserver"
	"sshc/internal/secret"
)

const (
	maxVaultResponseBody  = 64 << 10
	maxVaultRequestBody   = 4 << 10
	maxVaultPasswordBytes = 4 << 10
	vaultCommandTimeout   = 3 * time.Minute
)

// newVaultPasswordGuidance は、新しいマスターパスワードを尋ねる前に出す案内である。
// 最短の長さは、engine が確かめる secret.MinPassphraseLength から作る。
var newVaultPasswordGuidance = fmt.Sprintf("Enter at least %d characters, or press Enter without typing "+
	"a new password to use a passwordless vault. Leave the confirmation blank too.", secret.MinPassphraseLength)

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

// maskedPasswordTerminal is implemented by real OS terminals so secret prompts
// can confirm each hidden character as it is typed. Test doubles and alternative
// terminals may omit it; the caller then confirms the input after Enter.
type maskedPasswordTerminal interface {
	ReadPasswordMasked(
		ctx context.Context, input *os.File, prompt func() error, feedback func(int) error,
	) ([]byte, error)
}

type systemPasswordTerminal struct{}

func (systemPasswordTerminal) IsTerminal(fd int) bool { return term.IsTerminal(fd) }

// runVault は、起動済み engine の Vault operation だけを行う。engine を起動しない
// のは、CLI の補助コマンドが engine の持ち主にならないためである。engine は
// `sshc engine` か service が起動し、終了まで持ち続ける。
func runVault(ctx context.Context, action string, environment commandEnvironment) int {
	stateDir, client, stdin, stdout, stderr, terminal :=
		environment.stateDir, environment.client, environment.stdin, environment.stdout, environment.stderr, environment.terminal
	if err := ctx.Err(); err != nil {
		return exitInterrupted
	}
	if action != "status" && action != "create" && action != "unlock" &&
		action != "lock" && action != "change-password" {
		fmt.Fprintln(stderr, "sshc: unknown vault action")
		return exitUsage
	}

	needsPassword := action == "create" || action == "change-password"
	if needsPassword && (stdin == nil || terminal == nil || !terminal.IsTerminal(int(stdin.Fd()))) {
		fmt.Fprintln(stderr, "sshc: vault passwords require an interactive terminal")
		return exitFailure
	}

	found, err := verifiedHandoff(ctx, stateDir, client)
	if err != nil {
		return reportEngineUnreachable(ctx, err, stderr)
	}
	status, err := fetchVaultStatus(ctx, found, client)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return exitInterrupted
		}
		fmt.Fprintln(stderr, "sshc: the running engine did not return a valid vault status")
		return exitFailure
	}
	if ctx.Err() != nil {
		return exitInterrupted
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
			return exitFailure
		}
		return runVaultCreate(ctx, found, environment)
	case "unlock":
		if !status.Vault {
			fmt.Fprintln(stderr, "sshc: "+vaultMissingAdvice)
			return exitFailure
		}
		if status.Unlocked {
			fmt.Fprintln(stdout, "vault is already unlocked")
			return 0
		}
		if status.Passwordless {
			return finishVaultMutation(ctx, environment, vaultMutation{found: found, path: httpserver.VaultUnlockPath, payload: []byte(`{"passphrase":""}`), success: "vault unlocked"})
		}
		if stdin == nil || terminal == nil || !terminal.IsTerminal(int(stdin.Fd())) {
			fmt.Fprintln(stderr, "sshc: vault passwords require an interactive terminal")
			return exitFailure
		}
		return runVaultUnlock(ctx, found, environment)
	case "lock":
		if status.Passwordless {
			return finishVaultMutation(ctx, environment, vaultMutation{found: found, path: httpserver.VaultLockPath, payload: []byte("{}"), success: "passwordless vault remains unlocked; set a master password to enable locking"})
		}
		return runVaultLock(ctx, found, environment)
	case "change-password":
		if !status.Vault {
			fmt.Fprintln(stderr, "sshc: "+vaultMissingAdvice)
			return exitFailure
		}
		if !status.Unlocked && !status.Passwordless {
			fmt.Fprintln(stderr, "sshc: "+vaultLockedAdvice)
			return exitFailure
		}
		return runVaultChange(ctx, found, environment, status.Passwordless)
	default:
		return exitUsage
	}
}

func runVaultCreate(ctx context.Context, found handoff.Handoff, environment commandEnvironment) int {
	stdin, stderr, terminal := environment.stdin, environment.stderr, environment.terminal
	fmt.Fprintln(stderr, newVaultPasswordGuidance)
	next, err := promptVaultPassword(ctx, stdin, stderr, terminal, "New master password: ")
	defer clear(next)
	if err != nil {
		return vaultPromptFailure(ctx, err, stderr)
	}
	if ctx.Err() != nil {
		return exitInterrupted
	}
	confirmation, err := promptVaultPassword(ctx, stdin, stderr, terminal, "Confirm new master password: ")
	defer clear(confirmation)
	if err != nil {
		return vaultPromptFailure(ctx, err, stderr)
	}
	if ctx.Err() != nil {
		return exitInterrupted
	}
	if !bytes.Equal(next, confirmation) {
		fmt.Fprintln(stderr, "sshc: password confirmation did not match")
		return exitFailure
	}
	payload, err := vaultPassphrasePayload(next)
	if err != nil {
		fmt.Fprintln(stderr, "sshc: the password could not be encoded safely")
		return exitFailure
	}
	clear(next)
	clear(confirmation)
	return finishVaultMutation(ctx, environment, vaultMutation{found: found, path: httpserver.VaultCreatePath, payload: payload, success: "vault created and unlocked"})
}

func runVaultUnlock(ctx context.Context, found handoff.Handoff, environment commandEnvironment) int {
	stdin, stderr, terminal := environment.stdin, environment.stderr, environment.terminal
	password, err := promptVaultPassword(ctx, stdin, stderr, terminal, "Master password: ")
	defer clear(password)
	if err != nil {
		return vaultPromptFailure(ctx, err, stderr)
	}
	if ctx.Err() != nil {
		return exitInterrupted
	}
	payload, err := vaultPassphrasePayload(password)
	if err != nil {
		fmt.Fprintln(stderr, "sshc: the password could not be encoded safely")
		return exitFailure
	}
	clear(password)
	return finishVaultMutation(ctx, environment, vaultMutation{found: found, path: httpserver.VaultUnlockPath, payload: payload, success: "vault unlocked"})
}

func runVaultLock(ctx context.Context, found handoff.Handoff, environment commandEnvironment) int {
	return finishVaultMutation(ctx, environment, vaultMutation{found: found, path: httpserver.VaultLockPath, payload: []byte("{}"), success: "vault locked"})
}

func runVaultChange(ctx context.Context, found handoff.Handoff, environment commandEnvironment, passwordless bool) int {
	stdin, stderr, terminal := environment.stdin, environment.stderr, environment.terminal
	var current []byte
	var err error
	if !passwordless {
		current, err = promptVaultPassword(ctx, stdin, stderr, terminal, "Current master password: ")
	}
	defer clear(current)
	if err != nil {
		return vaultPromptFailure(ctx, err, stderr)
	}
	if ctx.Err() != nil {
		return exitInterrupted
	}
	payload, err := vaultPassphrasePayload(current)
	if err != nil {
		fmt.Fprintln(stderr, "sshc: the password could not be encoded safely")
		return exitFailure
	}
	if code := finishVaultMutation(ctx, environment, vaultMutation{found: found, path: httpserver.VaultVerifyPath, payload: payload, success: ""}); code != 0 {
		return code
	}
	fmt.Fprintln(stderr, newVaultPasswordGuidance)
	next, err := promptVaultPassword(ctx, stdin, stderr, terminal, "New master password: ")
	defer clear(next)
	if err != nil {
		return vaultPromptFailure(ctx, err, stderr)
	}
	if ctx.Err() != nil {
		return exitInterrupted
	}
	confirmation, err := promptVaultPassword(ctx, stdin, stderr, terminal, "Confirm new master password: ")
	defer clear(confirmation)
	if err != nil {
		return vaultPromptFailure(ctx, err, stderr)
	}
	if ctx.Err() != nil {
		return exitInterrupted
	}
	if !bytes.Equal(next, confirmation) {
		fmt.Fprintln(stderr, "sshc: password confirmation did not match")
		return exitFailure
	}
	payload, err = vaultChangePayload(current, next)
	if err != nil {
		fmt.Fprintln(stderr, "sshc: a password could not be encoded safely")
		return exitFailure
	}
	clear(current)
	clear(next)
	clear(confirmation)
	return finishVaultMutation(ctx, environment, vaultMutation{found: found, path: httpserver.VaultChangePath, payload: payload, success: "vault password changed"})
}

func promptVaultPassword(
	ctx context.Context, stdin *os.File, stderr io.Writer, terminal passwordTerminal, prompt string,
) ([]byte, error) {
	return promptMaskedPassword(ctx, stdin, stderr, terminal, prompt)
}

// promptMaskedPassword writes one asterisk per Unicode character while a real
// terminal reads without echo. The entered value itself is never written back.
func promptMaskedPassword(
	ctx context.Context, stdin *os.File, output io.Writer, terminal passwordTerminal, label string,
) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	prompted := false
	maskCount := 0
	promptInput := func() error {
		if _, err := fmt.Fprint(output, label); err != nil {
			return err
		}
		prompted = true
		return nil
	}
	feedback := func(next int) error {
		if next > maskCount {
			if _, err := fmt.Fprint(output, strings.Repeat("*", next-maskCount)); err != nil {
				return err
			}
		} else if next < maskCount {
			if _, err := fmt.Fprint(output, strings.Repeat("\b \b", maskCount-next)); err != nil {
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
			_, outputErr = fmt.Fprint(output, strings.Repeat("*", utf8.RuneCount(typed)))
		}
		if _, newlineErr := fmt.Fprintln(output); outputErr == nil {
			outputErr = newlineErr
		}
	}
	if err != nil {
		return typed, err
	}
	return typed, outputErr
}

func vaultPromptFailure(ctx context.Context, err error, stderr io.Writer) int {
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return exitInterrupted
	}
	fmt.Fprintln(stderr, "sshc: could not read the vault password")
	return exitFailure
}

// vaultMutation は engine の vault へ送る 1 つの変更。success は成功時に stdout へ
// 出す文で、空なら（verify のように）結果を表示しない。
type vaultMutation struct {
	found   handoff.Handoff
	path    string
	payload []byte
	success string
}

func finishVaultMutation(ctx context.Context, environment commandEnvironment, mutation vaultMutation) int {
	client, stdout, stderr := environment.client, environment.stdout, environment.stderr
	found, path, payload, success := mutation.found, mutation.path, mutation.payload, mutation.success
	if err := ctx.Err(); err != nil {
		clear(payload)
		return exitInterrupted
	}
	response, err := sendVaultPOST(ctx, client, found, path, payload)
	if err != nil {
		writeUncertainVaultResult(path, stderr)
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return exitInterrupted
		}
		return exitFailure
	}
	body, err := readAndCloseVaultResponse(response)
	defer clear(body)
	if err != nil {
		fmt.Fprintln(stderr, "sshc: the running engine returned an invalid vault response")
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return exitInterrupted
		}
		return exitFailure
	}
	if response.StatusCode == http.StatusNoContent {
		if success != "" {
			fmt.Fprintln(stdout, success)
		}
		return 0
	}
	switch response.StatusCode {
	case http.StatusNotFound:
		if path == httpserver.VaultVerifyPath {
			fmt.Fprintln(stderr, "sshc: the running engine does not support password verification; update and restart the engine, then try again")
		} else {
			fmt.Fprintln(stderr, "sshc: the vault operation failed")
		}
	case http.StatusUnauthorized:
		if path == httpserver.VaultUnlockPath || path == httpserver.VaultChangePath || path == httpserver.VaultVerifyPath {
			fmt.Fprintln(stderr, "sshc: the vault password or engine authentication was refused")
		} else {
			fmt.Fprintln(stderr, "sshc: engine authentication was refused")
		}
	case http.StatusConflict:
		if problem, ok := parseProblemBody(body); ok && problem.Code == httpserver.VaultBackupsTooManyCode {
			fmt.Fprintln(stderr, "sshc: there are too many local backups to re-encrypt in one change. Nothing was changed. "+
				"Delete old folders from ~/.ssh/sshc/backups and try again. Deleted backups can no longer be restored.")
		} else {
			fmt.Fprintln(stderr, "sshc: the vault state changed; run sshc vault status and try again")
		}
	case http.StatusBadRequest:
		fmt.Fprintln(stderr, "sshc: the vault password or request was not accepted")
	case http.StatusRequestEntityTooLarge:
		fmt.Fprintln(stderr, "sshc: the vault password or request is too large")
	default:
		fmt.Fprintln(stderr, "sshc: the vault operation failed")
	}
	return exitFailure
}

func writeUncertainVaultResult(path string, stderr io.Writer) {
	if path == httpserver.VaultVerifyPath {
		fmt.Fprintln(stderr, "sshc: current password verification did not complete; no password change was requested")
		return
	}
	if path == httpserver.VaultChangePath {
		fmt.Fprintln(stderr, "sshc: password change outcome is uncertain; the local password may already have changed. Run sshc vault status first; if it reports passwordless, no password is required. Otherwise run sshc vault lock (existing SSH sessions stay connected), then run sshc vault unlock with the new password first and the old password second.")
		return
	}
	fmt.Fprintln(stderr, "sshc: vault request outcome is uncertain; run sshc vault status to check the result")
}

func fetchVaultStatus(
	ctx context.Context, found handoff.Handoff, client *http.Client,
) (statusAnswer, error) {
	return fetchEngineStatus(ctx, client, found, httpserver.VaultStatusPath)
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
	defer clear(payload)
	return newHandoffEndpoint(found, client).send(ctx,
		handoffCall{method: http.MethodPost, path: path, body: &oneShotSecretPayload{body: payload}})
}

// vaultCommandClient は対話的な Vault 操作を短い接続確認タイムアウトから分離する。
// パスワード変更の engine 側は、ほかの sshc が workspace を持っていれば最長 30 秒
// 待ち、そのあと Vault、同期設定、すべての世代バックアップを再封印する。バックアップは
// 履歴とともに増え、遅いマシンでは数十秒かかりうるので、余裕を持たせる。リモートの
// スナップショットには書かない。キャンセルは引き続きリクエストコンテキストで伝播し、
// engine は錠を待つあいだに諦められた変更を始めない。
func vaultCommandClient(client *http.Client) *http.Client {
	if client == nil {
		client = http.DefaultClient
	}
	cloned := *client
	cloned.Timeout = vaultCommandTimeout
	return &cloned
}

func readAndCloseVaultResponse(response *http.Response) ([]byte, error) {
	return readAndCloseBounded(response, maxVaultResponseBody, errInvalidVaultResponse, errVaultResponseTooLarge)
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
		clear(payload)
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
		clear(payload)
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
