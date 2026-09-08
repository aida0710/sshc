package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"

	"sshc/internal/api"
)

type otpListEntry struct {
	Name  string   `json:"name"`
	Hosts []string `json:"hosts"`
}

func runOTP(
	ctx context.Context, called otpInvocation, stateDir string, client *http.Client,
	stdin *os.File, stdout, stderr io.Writer, terminal passwordTerminal,
) int {
	if err := ctx.Err(); err != nil {
		return finishOTPFailure(called.JSON, err, stdout, stderr)
	}
	if (called.Action == otpAdd || called.Action == otpEdit) &&
		(stdin == nil || terminal == nil || !terminal.IsTerminal(int(stdin.Fd()))) {
		fmt.Fprintln(stderr, "sshc: TOTP setup keys require an interactive terminal")
		return 1
	}
	engine, err := openEngineAPI(ctx, stateDir, client)
	if err != nil {
		return finishOTPFailure(called.JSON, err, stdout, stderr)
	}
	defer func() { _ = engine.Close() }()

	listed, err := listOTP(ctx, engine)
	if err != nil {
		return finishOTPFailure(called.JSON, err, stdout, stderr)
	}
	switch called.Action {
	case otpList:
		if called.JSON {
			return writeOTPJSON(stdout, listed)
		}
		if len(listed) == 0 {
			fmt.Fprintln(stdout, "No TOTP credentials are stored.")
			return 0
		}
		for _, entry := range listed {
			hosts := "not assigned"
			if len(entry.Hosts) > 0 {
				hosts = strings.Join(entry.Hosts, ", ")
			}
			fmt.Fprintf(stdout, "%s\t%s\n", safeTerminalCell(entry.Name), safeTerminalCell(hosts))
		}
		return 0
	case otpShow:
		if !hasOTP(listed, called.Name) {
			return finishOTPFailure(called.JSON, engineProblem{Status: http.StatusNotFound, Code: "unknown_credential"}, stdout, stderr)
		}
		codes, err := generateOTPCodes(ctx, engine, called.Name)
		if err != nil {
			return finishOTPFailure(called.JSON, err, stdout, stderr)
		}
		if called.JSON {
			return writeOTPJSON(stdout, codes)
		}
		fmt.Fprintf(stdout, "previous  %s\ncurrent   %s  (%ds remaining)\nnext      %s\n",
			codes.Previous, codes.Current, codes.RemainingSeconds, codes.Next)
		return 0
	case otpAdd, otpEdit:
		exists := hasOTP(listed, called.Name)
		if called.Action == otpAdd && exists {
			fmt.Fprintln(stderr, "sshc: a TOTP credential with that name already exists; use `sshc otp edit`")
			return 1
		}
		if called.Action == otpEdit && !exists {
			fmt.Fprintln(stderr, "sshc: no TOTP credential has that name; use `sshc otp add`")
			return 1
		}
		setup, err := promptMaskedPassword(ctx, stdin, stderr, terminal, "TOTP setup key or otpauth URI: ")
		defer zeroBytes(setup)
		if err != nil {
			if errors.Is(ctx.Err(), context.Canceled) {
				return 130
			}
			fmt.Fprintln(stderr, "sshc: could not read the TOTP setup key")
			return 1
		}
		payload, err := json.Marshal(api.StoreCredentialRequest{Secret: string(setup)})
		if err != nil {
			fmt.Fprintln(stderr, "sshc: could not encode the TOTP setup key safely")
			return 1
		}
		zeroBytes(setup)
		var updated api.CredentialList
		path := "/api/v1/credentials/totp/" + url.PathEscape(called.Name)
		if err := engine.sendSecretJSON(ctx, http.MethodPut, path, payload, &updated); err != nil {
			return finishOTPFailure(false, err, stdout, stderr)
		}
		verb := "added"
		if called.Action == otpEdit {
			verb = "updated"
		}
		fmt.Fprintf(stdout, "TOTP %s: %s\n", verb, safeTerminalCell(called.Name))
		return 0
	case otpRemove:
		entry, found := findOTP(listed, called.Name)
		if !found {
			fmt.Fprintln(stderr, "sshc: no TOTP credential has that name")
			return 1
		}
		if len(entry.Hosts) > 0 {
			fmt.Fprintf(stderr, "sshc: TOTP is still assigned to: %s\n", safeTerminalCell(strings.Join(entry.Hosts, ", ")))
			fmt.Fprintln(stderr, "sshc: remove those assignments from Connections before deleting it")
			return 1
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
		if err := engine.sendJSON(ctx, http.MethodDelete,
			"/api/v1/credentials/totp/"+url.PathEscape(called.Name), nil, &updated); err != nil {
			return finishOTPFailure(false, err, stdout, stderr)
		}
		fmt.Fprintf(stdout, "TOTP removed: %s\n", safeTerminalCell(called.Name))
		return 0
	default:
		fmt.Fprintln(stderr, "sshc: unknown otp action")
		return 2
	}
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

func writeOTPJSON(out io.Writer, value any) int {
	if err := json.NewEncoder(out).Encode(value); err != nil {
		return 1
	}
	return 0
}

func finishOTPFailure(asJSON bool, err error, stdout, stderr io.Writer) int {
	exit := 1
	if errors.Is(err, context.Canceled) {
		exit = 130
	}
	failure := classifyCommandFailure(err)
	if asJSON {
		_ = writeCommandEnvelope(stdout, commandEnvelope{
			SchemaVersion: 1, Success: false, Failure: &failure,
		})
		return exit
	}
	var problem engineProblem
	switch {
	case errors.Is(err, fs.ErrNotExist):
		fmt.Fprintln(stderr, "sshc: no engine is running; start the desktop app or run sshc engine")
	case errors.Is(err, errEngineVaultMissing):
		fmt.Fprintln(stderr, "sshc: no vault exists; run sshc vault create")
	case errors.Is(err, errEngineVaultLocked):
		fmt.Fprintln(stderr, "sshc: the vault is locked; run sshc vault unlock")
	case errors.As(err, &problem) && problem.Code == "unknown_credential":
		fmt.Fprintln(stderr, "sshc: no TOTP credential has that name")
	case errors.As(err, &problem) && problem.Code == "invalid_request":
		fmt.Fprintln(stderr, "sshc: the TOTP name or setup key is invalid")
	case errors.As(err, &problem) && problem.Code == "credential_in_use":
		fmt.Fprintln(stderr, "sshc: the TOTP is still assigned to a connection")
	case errors.Is(err, context.Canceled):
		fmt.Fprintln(stderr, "sshc: OTP operation was canceled")
	default:
		fmt.Fprintln(stderr, "sshc: the running engine could not complete the OTP operation")
	}
	return exit
}
