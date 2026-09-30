package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"sshc/internal/api"
)

type otpListEntry struct {
	Name  string   `json:"name"`
	Hosts []string `json:"hosts"`
}

func runOTP(ctx context.Context, called otpInvocation, environment commandEnvironment) int {
	stdin, stdout, stderr, terminal := environment.stdin, environment.stdout, environment.stderr, environment.terminal
	if err := ctx.Err(); err != nil {
		return finishOTPFailure(called.JSON, err, stdout, stderr)
	}
	if (called.Action == otpAdd || called.Action == otpEdit) &&
		(stdin == nil || terminal == nil || !terminal.IsTerminal(int(stdin.Fd()))) {
		fmt.Fprintln(stderr, "sshc: TOTP setup keys require an interactive terminal")
		return exitFailure
	}
	engine, err := openEngineAPI(ctx, environment.stateDir, environment.client)
	if err != nil {
		return finishOTPFailure(called.JSON, err, stdout, stderr)
	}
	defer func() { _ = engine.Close() }()

	listed, err := listOTP(ctx, engine)
	if err != nil {
		return finishOTPFailure(called.JSON, err, stdout, stderr)
	}
	run := otpRun{called: called, environment: environment, engine: engine, listed: listed}
	switch called.Action {
	case otpList:
		return run.list()
	case otpShow:
		return run.show(ctx)
	case otpAdd, otpEdit:
		return run.save(ctx)
	case otpRemove:
		return run.remove(ctx)
	default:
		fmt.Fprintln(stderr, "sshc: unknown otp action")
		return exitUsage
	}
}

// otpRun は、engine に繋いで保存済みの TOTP を読んだあとの、`sshc otp` の 1 回の実行である。
type otpRun struct {
	called      otpInvocation
	environment commandEnvironment
	engine      *engineAPI
	// listed は、engine に保存されている TOTP の一覧である。
	listed []otpListEntry
}

// list は、保存済みの TOTP と、それを使う接続を出す。
func (run otpRun) list() int {
	stdout, stderr := run.environment.stdout, run.environment.stderr
	if run.called.JSON {
		return writeOTPJSON(stdout, stderr, run.listed)
	}
	if len(run.listed) == 0 {
		fmt.Fprintln(stdout, "No TOTP credentials are stored.")
		return 0
	}
	for _, entry := range run.listed {
		hosts := "not assigned"
		if len(entry.Hosts) > 0 {
			hosts = strings.Join(entry.Hosts, ", ")
		}
		fmt.Fprintf(stdout, "%s\t%s\n", safeTerminalCell(entry.Name), safeTerminalCell(hosts))
	}
	return 0
}

// show は、前・今・次のコードを出す。
func (run otpRun) show(ctx context.Context) int {
	called, stdout, stderr := run.called, run.environment.stdout, run.environment.stderr
	if !hasOTP(run.listed, called.Name) {
		return finishOTPFailure(called.JSON, engineProblem{Status: http.StatusNotFound, Code: "unknown_credential"}, stdout, stderr)
	}
	codes, err := generateOTPCodes(ctx, run.engine, called.Name)
	if err != nil {
		return finishOTPFailure(called.JSON, err, stdout, stderr)
	}
	if called.JSON {
		return writeOTPJSON(stdout, stderr, codes)
	}
	fmt.Fprintf(stdout, "previous  %s\ncurrent   %s  (%ds remaining)\nnext      %s\n",
		codes.Previous, codes.Current, codes.RemainingSeconds, codes.Next)
	return 0
}

// save は、画面に出さずに読んだ設定キーを、add なら新しい名前で、edit なら既存の
// 名前へ保存する。
func (run otpRun) save(ctx context.Context) int {
	called, environment := run.called, run.environment
	stdout, stderr := environment.stdout, environment.stderr
	exists := hasOTP(run.listed, called.Name)
	if called.Action == otpAdd && exists {
		fmt.Fprintln(stderr, "sshc: a TOTP credential with that name already exists; use `sshc otp edit`")
		return exitFailure
	}
	if called.Action == otpEdit && !exists {
		fmt.Fprintln(stderr, "sshc: no TOTP credential has that name; use `sshc otp add`")
		return exitFailure
	}
	setup, err := promptMaskedPassword(ctx, environment.stdin, stderr, environment.terminal, "TOTP setup key or otpauth URI: ")
	defer zeroBytes(setup)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return exitInterrupted
		}
		fmt.Fprintln(stderr, "sshc: could not read the TOTP setup key")
		return exitFailure
	}
	payload, err := json.Marshal(api.StoreCredentialRequest{Secret: string(setup)})
	if err != nil {
		fmt.Fprintln(stderr, "sshc: could not encode the TOTP setup key safely")
		return exitFailure
	}
	zeroBytes(setup)
	var updated api.CredentialList
	path := "/api/v1/credentials/totp/" + url.PathEscape(called.Name)
	if err := run.engine.sendSecretJSON(ctx, http.MethodPut, path, payload, &updated); err != nil {
		return finishOTPFailure(false, err, stdout, stderr)
	}
	verb := "added"
	if called.Action == otpEdit {
		verb = "updated"
	}
	fmt.Fprintf(stdout, "TOTP %s: %s\n", verb, safeTerminalCell(called.Name))
	return 0
}

// remove は、どの接続にも割り当てられていない TOTP を、確認のあとで消す。
func (run otpRun) remove(ctx context.Context) int {
	called, stdout, stderr := run.called, run.environment.stdout, run.environment.stderr
	entry, found := findOTP(run.listed, called.Name)
	if !found {
		fmt.Fprintln(stderr, "sshc: no TOTP credential has that name")
		return exitFailure
	}
	if len(entry.Hosts) > 0 {
		fmt.Fprintf(stderr, "sshc: TOTP is still assigned to: %s\n", safeTerminalCell(strings.Join(entry.Hosts, ", ")))
		fmt.Fprintln(stderr, "sshc: remove those assignments from Connections before deleting it")
		return exitFailure
	}
	confirmed, exit := confirmAction(ctx, called.Yes,
		fmt.Sprintf("Remove saved TOTP %q? [y/N] ", safeTerminalCell(called.Name)),
		systemActionConfirmer, stderr)
	if exit != 0 {
		return exit
	}
	if !confirmed {
		fmt.Fprintln(stdout, "No changes made.")
		return 0
	}
	var updated api.CredentialList
	if err := run.engine.sendJSON(ctx, http.MethodDelete,
		"/api/v1/credentials/totp/"+url.PathEscape(called.Name), nil, &updated); err != nil {
		return finishOTPFailure(false, err, stdout, stderr)
	}
	fmt.Fprintf(stdout, "TOTP removed: %s\n", safeTerminalCell(called.Name))
	return 0
}

func listOTP(ctx context.Context, engine *engineAPI) ([]otpListEntry, error) {
	var response api.CredentialList
	if err := engine.getJSON(ctx, "/api/v1/credentials", &response); err != nil {
		return nil, err
	}
	listed := make([]otpListEntry, 0)
	for _, credential := range response.Credentials {
		if credential.Kind != api.Totp {
			continue
		}
		listed = append(listed, otpListEntry{
			Name: credential.Name, Hosts: append([]string{}, credential.Hosts...),
		})
	}
	sort.Slice(listed, func(left, right int) bool { return listed[left].Name < listed[right].Name })
	return listed, nil
}

func findOTP(listed []otpListEntry, name string) (otpListEntry, bool) {
	for _, entry := range listed {
		if entry.Name == name {
			return entry, true
		}
	}
	return otpListEntry{}, false
}

func hasOTP(listed []otpListEntry, name string) bool {
	_, found := findOTP(listed, name)
	return found
}

func generateOTPCodes(ctx context.Context, engine *engineAPI, name string) (api.TOTPCodeSet, error) {
	target := "totp\n" + name
	action, err := engine.issueAction(ctx, "credential.reveal", target)
	if err != nil {
		return api.TOTPCodeSet{}, err
	}
	var codes api.TOTPCodeSet
	err = engine.sendJSONWithAction(ctx, http.MethodPost,
		"/api/v1/credentials/totp/"+url.PathEscape(name)+"/codes",
		action.Token, nil, &codes)
	return codes, err
}

func writeOTPJSON(stdout, stderr io.Writer, result any) int {
	if err := writeCommandSuccess(stdout, result); err != nil {
		fmt.Fprintln(stderr, "sshc: could not write JSON result")
		return exitFailure
	}
	return 0
}

func finishOTPFailure(asJSON bool, err error, stdout, stderr io.Writer) int {
	return finishCommandFailure(commandFailureReport{
		cause: err, failure: classifyCommandFailure(err),
		asJSON: asJSON, stdout: stdout, stderr: stderr, writeHuman: writeHumanOTPFailure,
	})
}

// writeHumanOTPFailure は、OTP 固有の失敗の種別を人向けの文で書く。
func writeHumanOTPFailure(stderr io.Writer, failure commandFailure) {
	switch failure.Kind {
	case "unknown_credential":
		fmt.Fprintln(stderr, "sshc: no TOTP credential has that name")
	case "invalid_request":
		fmt.Fprintln(stderr, "sshc: the TOTP name or setup key is invalid")
	case "credential_in_use":
		fmt.Fprintln(stderr, "sshc: the TOTP is still assigned to a connection")
	case "canceled":
		fmt.Fprintln(stderr, "sshc: OTP operation was canceled")
	default:
		fmt.Fprintln(stderr, "sshc: the running engine could not complete the OTP operation")
	}
}
