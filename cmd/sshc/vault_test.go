package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"sshc/internal/handoff"
	"sshc/internal/httpserver"
)

const vaultPasswordCanary = "vault-password-canary-123"

type fakePasswordTerminal struct {
	terminal  bool
	answers   [][]byte
	errors    []error
	reads     int
	afterRead func(index int)
}

type orderedVaultPromptWriter struct {
	events *[]string
}

func (w orderedVaultPromptWriter) Write(value []byte) (int, error) {
	*w.events = append(*w.events, "write:"+string(value))
	return len(value), nil
}

type orderedVaultPromptTerminal struct {
	events *[]string
}

func (orderedVaultPromptTerminal) IsTerminal(int) bool { return true }

func (t orderedVaultPromptTerminal) ReadPassword(
	_ context.Context, _ *os.File, prompt func() error,
) ([]byte, error) {
	*t.events = append(*t.events, "no-echo")
	if err := prompt(); err != nil {
		return nil, err
	}
	*t.events = append(*t.events, "read-and-restore")
	return []byte("master"), nil
}

func (f *fakePasswordTerminal) IsTerminal(int) bool { return f.terminal }

func (f *fakePasswordTerminal) ReadPassword(
	_ context.Context, _ *os.File, prompt func() error,
) ([]byte, error) {
	if err := prompt(); err != nil {
		return nil, err
	}
	index := f.reads
	f.reads++
	if f.afterRead != nil {
		f.afterRead(index)
	}
	if index < len(f.errors) && f.errors[index] != nil {
		if index < len(f.answers) {
			return f.answers[index], f.errors[index]
		}
		return nil, f.errors[index]
	}
	if index >= len(f.answers) {
		return nil, errors.New("unexpected password read")
	}
	return f.answers[index], nil
}

func vaultTestInput(t *testing.T) *os.File {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "vault-tty")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file
}

func TestVaultPromptIsWrittenAfterNoEchoAndNewlineAfterRestore(t *testing.T) {
	events := make([]string, 0, 4)
	password, err := promptVaultPassword(context.Background(), vaultTestInput(t),
		orderedVaultPromptWriter{events: &events}, orderedVaultPromptTerminal{events: &events}, "Master password: ")
	defer zeroBytes(password)
	if err != nil || !bytes.Equal(password, []byte("master")) {
		t.Fatalf("password=%q error=%v", password, err)
	}
	want := []string{"no-echo", "write:Master password: ", "read-and-restore", "write:\n"}
	if !slices.Equal(events, want) {
		t.Fatalf("events=%q, want %q", events, want)
	}
}

func writeVaultTestHandoff(t *testing.T, stateDir, target string, owner handoff.Owner) handoff.Handoff {
	t.Helper()
	document := testHandoff(target)
	document.Owner = owner
	document.Version = "v4-test"
	if err := handoff.Write(stateDir, document); err != nil {
		t.Fatal(err)
	}
	return document
}

func vaultStatusBody(owner handoff.Owner, vault, unlocked bool) string {
	encoded, _ := json.Marshal(map[string]any{
		"owner": owner, "version": "v4-test", "protocolVersion": handoff.ProtocolVersion,
		"vault": vault, "unlocked": unlocked, "sessions": 2,
	})
	return string(encoded)
}

func TestRunVaultStatusIsHumanReadableWithoutATerminal(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests++
		if request.Method != http.MethodGet || request.URL.Path != httpserver.VaultStatusPath ||
			request.Header.Get(handoff.HeaderName) != "the secret" {
			response.WriteHeader(http.StatusUnauthorized)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(response, vaultStatusBody(handoff.OwnerEngine, true, false))
	}))
	defer server.Close()
	stateDir := t.TempDir()
	writeVaultTestHandoff(t, stateDir, server.URL, handoff.OwnerEngine)

	terminal := &fakePasswordTerminal{terminal: false}
	var stdout, stderr strings.Builder
	code := runVault(context.Background(), "status", stateDir, server.Client(), vaultTestInput(t), &stdout, &stderr, terminal)
	if code != 0 {
		t.Fatalf("runVault status = %d, stderr = %q", code, stderr.String())
	}
	// `sshc status` と同じ表である。描き手は writeStatus ひとつなので、
	// ここで見るのは「その表が出ていること」であって書式そのものではない。
	//
	// 表記を丸ごと比べない。address は毎回違う番号を持つ。
	printed := stdout.String()
	for _, want := range []string{"version   v4-test", "vault     locked", "consoles  2", "address   " + server.URL} {
		if !strings.Contains(printed, want) {
			t.Errorf("stdout does not contain %q:\n%s", want, printed)
		}
	}
	if stderr.String() != "" || terminal.reads != 0 || requests != 1 {
		t.Fatalf("stderr=%q reads=%d requests=%d", stderr.String(), terminal.reads, requests)
	}
}

func TestRunVaultCreateChecksStateBeforePrompting(t *testing.T) {
	var posts int
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost {
			posts++
		}
		_, _ = io.WriteString(response, vaultStatusBody(handoff.OwnerEngine, true, false))
	}))
	defer server.Close()
	stateDir := t.TempDir()
	writeVaultTestHandoff(t, stateDir, server.URL, handoff.OwnerEngine)

	terminal := &fakePasswordTerminal{terminal: true}
	var stdout, stderr strings.Builder
	code := runVault(context.Background(), "create", stateDir, server.Client(), vaultTestInput(t), &stdout, &stderr, terminal)
	if code != 1 || terminal.reads != 0 || posts != 0 {
		t.Fatalf("code=%d reads=%d posts=%d stderr=%q", code, terminal.reads, posts, stderr.String())
	}
	if strings.Contains(stdout.String()+stderr.String(), vaultPasswordCanary) {
		t.Fatal("output leaked a password")
	}
}

func TestRunVaultRefusesPasswordActionsWithoutATerminalBeforeAnyRequest(t *testing.T) {
	for _, action := range []string{"create", "unlock", "change-password"} {
		t.Run(action, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
			defer server.Close()
			stateDir := t.TempDir()
			writeVaultTestHandoff(t, stateDir, server.URL, handoff.OwnerEngine)

			var stdout, stderr strings.Builder
			code := runVault(context.Background(), action, stateDir, server.Client(), vaultTestInput(t), &stdout, &stderr,
				&fakePasswordTerminal{terminal: false})
			if code != 1 || requests != 0 || stdout.String() != "" {
				t.Fatalf("code=%d requests=%d stdout=%q stderr=%q", code, requests, stdout.String(), stderr.String())
			}
		})
	}
}

func TestRunVaultConfirmationMismatchSendsNoMutation(t *testing.T) {
	for _, action := range []string{"create", "change-password"} {
		t.Run(action, func(t *testing.T) {
			posts := 0
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodPost {
					posts++
				}
				vaultExists := action == "change-password"
				_, _ = io.WriteString(response, vaultStatusBody(handoff.OwnerEngine, vaultExists, vaultExists))
			}))
			defer server.Close()
			stateDir := t.TempDir()
			writeVaultTestHandoff(t, stateDir, server.URL, handoff.OwnerEngine)

			answers := [][]byte{[]byte(vaultPasswordCanary), []byte("different confirmation")}
			if action == "change-password" {
				answers = [][]byte{[]byte("current password"), []byte(vaultPasswordCanary), []byte("different confirmation")}
			}
			original := append([][]byte(nil), answers...)
			terminal := &fakePasswordTerminal{terminal: true, answers: answers}
			var stdout, stderr strings.Builder
			code := runVault(context.Background(), action, stateDir, server.Client(), vaultTestInput(t), &stdout, &stderr, terminal)
			if code != 1 || posts != 0 {
				t.Fatalf("code=%d posts=%d stderr=%q", code, posts, stderr.String())
			}
			for index, typed := range original {
				if !allZero(typed) {
					t.Fatalf("typed password %d was not erased: %q", index, typed)
				}
			}
			if strings.Contains(stdout.String()+stderr.String(), vaultPasswordCanary) {
				t.Fatal("mismatch output leaked a password")
			}
		})
	}
}

func TestRunVaultUnlockSkipsPromptWhenAlreadyUnlocked(t *testing.T) {
	var posts int
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost {
			posts++
		}
		_, _ = io.WriteString(response, vaultStatusBody(handoff.OwnerEngine, true, true))
	}))
	defer server.Close()
	stateDir := t.TempDir()
	writeVaultTestHandoff(t, stateDir, server.URL, handoff.OwnerEngine)

	terminal := &fakePasswordTerminal{terminal: true}
	var stdout, stderr strings.Builder
	code := runVault(context.Background(), "unlock", stateDir, server.Client(), vaultTestInput(t), &stdout, &stderr, terminal)
	if code != 0 || terminal.reads != 0 || posts != 0 {
		t.Fatalf("code=%d reads=%d posts=%d stdout=%q stderr=%q", code, terminal.reads, posts, stdout.String(), stderr.String())
	}
}

func TestRunVaultLockAuthenticationFailureDoesNotBlameAPassword(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet && request.URL.Path == httpserver.VaultStatusPath {
			_, _ = io.WriteString(response, vaultStatusBody(handoff.OwnerEngine, true, true))
			return
		}
		if request.Method == http.MethodPost && request.URL.Path == httpserver.VaultLockPath {
			response.WriteHeader(http.StatusUnauthorized)
			return
		}
		t.Fatalf("request = %s %s", request.Method, request.URL.Path)
	}))
	defer server.Close()
	stateDir := t.TempDir()
	writeVaultTestHandoff(t, stateDir, server.URL, handoff.OwnerEngine)

	var stdout, stderr strings.Builder
	code := runVault(context.Background(), "lock", stateDir, server.Client(), vaultTestInput(t), &stdout, &stderr,
		&fakePasswordTerminal{terminal: false})
	if code != 1 || !strings.Contains(stderr.String(), "engine authentication") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if strings.Contains(stderr.String(), "vault password") {
		t.Fatalf("password-free lock blamed a password: %q", stderr.String())
	}
}

func TestRunVaultLockValidatesLiveIdentityBeforeMutation(t *testing.T) {
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost {
			posts++
			response.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = io.WriteString(response, vaultStatusBody(handoff.Owner("unknown"), true, true))
	}))
	defer server.Close()
	stateDir := t.TempDir()
	writeVaultTestHandoff(t, stateDir, server.URL, handoff.OwnerEngine)

	var stdout, stderr strings.Builder
	code := runVault(context.Background(), "lock", stateDir, server.Client(), vaultTestInput(t), &stdout, &stderr,
		&fakePasswordTerminal{terminal: false})
	if code != 1 || posts != 0 || stdout.String() != "" {
		t.Fatalf("code=%d posts=%d stdout=%q stderr=%q", code, posts, stdout.String(), stderr.String())
	}
}

func TestRunVaultLockUsesAuthenticatedSessionPreservingRoute(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests++
		if request.Header.Get(handoff.HeaderName) != "the secret" {
			response.WriteHeader(http.StatusUnauthorized)
			return
		}
		if request.Method == http.MethodGet && request.URL.Path == httpserver.VaultStatusPath {
			_, _ = io.WriteString(response, vaultStatusBody(handoff.OwnerEngine, true, true))
			return
		}
		if request.Method == http.MethodPost && request.URL.Path == httpserver.VaultLockPath {
			body, err := io.ReadAll(request.Body)
			if err != nil || !bytes.Equal(body, []byte("{}")) {
				response.WriteHeader(http.StatusBadRequest)
				return
			}
			response.WriteHeader(http.StatusNoContent)
			return
		}
		response.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	stateDir := t.TempDir()
	writeVaultTestHandoff(t, stateDir, server.URL, handoff.OwnerEngine)
	terminal := &fakePasswordTerminal{terminal: false}

	var stdout, stderr strings.Builder
	code := runVault(context.Background(), "lock", stateDir, server.Client(), vaultTestInput(t), &stdout, &stderr, terminal)
	if code != 0 || requests != 2 || terminal.reads != 0 || stdout.String() != "vault locked\n" || stderr.String() != "" {
		t.Fatalf("code=%d requests=%d reads=%d stdout=%q stderr=%q",
			code, requests, terminal.reads, stdout.String(), stderr.String())
	}
}

type requestInspectingTransport struct {
	mu             sync.Mutex
	requests       int
	getBodyPresent bool
	payload        []byte
	status         int
	body           string
}

type vaultTransportError struct {
	message []byte
}

func (e vaultTransportError) Error() string { return string(e.message) }

type statusThenErrorTransport struct {
	requests int
	error    error
	status   string
}

func (t *statusThenErrorTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	t.requests++
	if request.Method == http.MethodGet {
		status := t.status
		if status == "" {
			status = vaultStatusBody(handoff.OwnerEngine, true, false)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(status)),
			Request:    request,
		}, nil
	}
	_, _ = io.ReadAll(request.Body)
	return nil, t.error
}

func TestVaultCommandClientAllowsLocalReencryptionToFinish(t *testing.T) {
	base := &http.Client{Timeout: 5 * time.Second}
	got := vaultCommandClient(base)
	if got == base {
		t.Fatal("vault client mutated the shared HTTP client")
	}
	if base.Timeout != 5*time.Second {
		t.Fatalf("shared timeout = %v, want 5s", base.Timeout)
	}
	if got.Timeout != 3*time.Minute {
		t.Fatalf("vault timeout = %v, want 3m", got.Timeout)
	}
}

func TestRunVaultExplainsUncertainPasswordChangeAfterTimeoutOrCancel(t *testing.T) {
	for _, test := range []struct {
		name     string
		err      error
		wantCode int
	}{
		{name: "timeout", err: context.DeadlineExceeded, wantCode: 1},
		{name: "cancel", err: context.Canceled, wantCode: 130},
	} {
		t.Run(test.name, func(t *testing.T) {
			transport := &statusThenErrorTransport{
				error:  test.err,
				status: vaultStatusBody(handoff.OwnerEngine, true, true),
			}
			stateDir := t.TempDir()
			writeVaultTestHandoff(t, stateDir, "http://127.0.0.1:42805", handoff.OwnerEngine)
			current := []byte("current password")
			next := []byte(vaultPasswordCanary)
			confirmation := []byte(vaultPasswordCanary)

			var stdout, stderr strings.Builder
			code := runVault(context.Background(), "change-password", stateDir, &http.Client{Transport: transport},
				vaultTestInput(t), &stdout, &stderr,
				&fakePasswordTerminal{terminal: true, answers: [][]byte{current, next, confirmation}})
			if code != test.wantCode || transport.requests != 2 {
				t.Fatalf("code=%d requests=%d stdout=%q stderr=%q", code, transport.requests, stdout.String(), stderr.String())
			}
			for _, phrase := range []string{
				"local password may already have changed",
				"sshc vault lock",
				"new password first",
				"old password second",
			} {
				if !strings.Contains(stderr.String(), phrase) {
					t.Fatalf("stderr=%q, want phrase %q", stderr.String(), phrase)
				}
			}
			if strings.Contains(stdout.String()+stderr.String(), vaultPasswordCanary) {
				t.Fatal("uncertain-result guidance leaked a password")
			}
			if !allZero(current) || !allZero(next) || !allZero(confirmation) {
				t.Fatal("change-password buffers were not erased after an uncertain result")
			}
		})
	}
}

func TestRunVaultExplainsHowToCheckUncertainCreateOrUnlock(t *testing.T) {
	for _, test := range []struct {
		action string
		status string
		reads  int
	}{
		{action: "create", status: vaultStatusBody(handoff.OwnerEngine, false, false), reads: 2},
		{action: "unlock", status: vaultStatusBody(handoff.OwnerEngine, true, false), reads: 1},
	} {
		t.Run(test.action, func(t *testing.T) {
			transport := &statusThenErrorTransport{error: context.DeadlineExceeded, status: test.status}
			stateDir := t.TempDir()
			writeVaultTestHandoff(t, stateDir, "http://127.0.0.1:42806", handoff.OwnerEngine)
			answers := make([][]byte, test.reads)
			for index := range answers {
				answers[index] = []byte(vaultPasswordCanary)
			}

			var stdout, stderr strings.Builder
			code := runVault(context.Background(), test.action, stateDir, &http.Client{Transport: transport},
				vaultTestInput(t), &stdout, &stderr,
				&fakePasswordTerminal{terminal: true, answers: answers})
			if code != 1 || transport.requests != 2 || !strings.Contains(stderr.String(), "sshc vault status") {
				t.Fatalf("code=%d requests=%d stdout=%q stderr=%q", code, transport.requests, stdout.String(), stderr.String())
			}
			if strings.Contains(stdout.String()+stderr.String(), vaultPasswordCanary) {
				t.Fatal("uncertain-result guidance leaked a password")
			}
			for _, answer := range answers {
				if !allZero(answer) {
					t.Fatal("password buffer was not erased after an uncertain result")
				}
			}
		})
	}
}

func (t *requestInspectingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.requests++
	t.getBodyPresent = request.GetBody != nil
	if request.Body != nil {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		t.payload = body
	}
	return &http.Response{
		StatusCode: t.status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(t.body)),
		Request:    request,
	}, nil
}

func TestSendVaultPOSTErasesPayloadAndDoesNotMakeItReplayable(t *testing.T) {
	transport := &requestInspectingTransport{status: http.StatusNoContent}
	client := &http.Client{Transport: transport}
	payload := []byte(`{"passphrase":"` + vaultPasswordCanary + `"}`)
	found := testHandoff("http://127.0.0.1:42800")

	response, err := sendVaultPOST(context.Background(), client, found, httpserver.VaultUnlockPath, payload)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if transport.getBodyPresent {
		t.Fatal("vault request body can be replayed")
	}
	if !allZero(payload) {
		t.Fatalf("JSON payload was not erased: %q", payload)
	}
	if !bytes.Contains(transport.payload, []byte(vaultPasswordCanary)) {
		t.Fatalf("transport did not receive the encoded password: %q", transport.payload)
	}
}

func TestSendVaultPOSTErasesPayloadOnTransportError(t *testing.T) {
	transport := &statusThenErrorTransport{error: vaultTransportError{message: []byte("reflected " + vaultPasswordCanary)}}
	payload := []byte(`{"passphrase":"` + vaultPasswordCanary + `"}`)
	found := testHandoff("http://127.0.0.1:42807")

	response, err := sendVaultPOST(context.Background(), &http.Client{Transport: transport}, found,
		httpserver.VaultUnlockPath, payload)
	if err == nil || response != nil {
		t.Fatalf("response=%v err=%v, want transport error", response, err)
	}
	if !allZero(payload) {
		t.Fatalf("JSON payload was not erased after transport error: %q", payload)
	}
}

func TestRunVaultDoesNotPrintATransportErrorThatReflectsThePassword(t *testing.T) {
	transport := &statusThenErrorTransport{error: vaultTransportError{message: []byte("transport reflected " + vaultPasswordCanary)}}
	client := &http.Client{Transport: transport}
	stateDir := t.TempDir()
	writeVaultTestHandoff(t, stateDir, "http://127.0.0.1:42802", handoff.OwnerEngine)
	typed := []byte(vaultPasswordCanary)

	var stdout, stderr strings.Builder
	code := runVault(context.Background(), "unlock", stateDir, client, vaultTestInput(t), &stdout, &stderr,
		&fakePasswordTerminal{terminal: true, answers: [][]byte{typed}})
	if code != 1 || transport.requests != 2 {
		t.Fatalf("code=%d requests=%d stdout=%q stderr=%q", code, transport.requests, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String()+stderr.String(), vaultPasswordCanary) {
		t.Fatal("transport error leaked the password")
	}
	if !allZero(typed) {
		t.Fatal("typed password was not erased after transport error")
	}
}

func TestRunVaultDoesNotPrintANonSuccessBodyThatReflectsThePassword(t *testing.T) {
	typed := []byte(vaultPasswordCanary)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			_, _ = io.WriteString(response, vaultStatusBody(handoff.OwnerEngine, true, false))
			return
		}
		response.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(response, "server reflected "+vaultPasswordCanary)
	}))
	defer server.Close()
	stateDir := t.TempDir()
	writeVaultTestHandoff(t, stateDir, server.URL, handoff.OwnerEngine)

	var stdout, stderr strings.Builder
	code := runVault(context.Background(), "unlock", stateDir, server.Client(), vaultTestInput(t), &stdout, &stderr,
		&fakePasswordTerminal{terminal: true, answers: [][]byte{typed}})
	if code != 1 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String()+stderr.String(), vaultPasswordCanary) {
		t.Fatal("non-success response leaked the password")
	}
	if !allZero(typed) {
		t.Fatal("typed password was not erased after non-success response")
	}
}

func TestRunVaultExplainsAnOversizedPasswordRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			_, _ = io.WriteString(response, vaultStatusBody(handoff.OwnerEngine, true, false))
			return
		}
		response.WriteHeader(http.StatusRequestEntityTooLarge)
	}))
	defer server.Close()
	stateDir := t.TempDir()
	writeVaultTestHandoff(t, stateDir, server.URL, handoff.OwnerEngine)
	typed := []byte(vaultPasswordCanary)

	var stdout, stderr strings.Builder
	code := runVault(context.Background(), "unlock", stateDir, server.Client(), vaultTestInput(t), &stdout, &stderr,
		&fakePasswordTerminal{terminal: true, answers: [][]byte{typed}})
	if code != 1 || !strings.Contains(stderr.String(), "too large") || !allZero(typed) {
		t.Fatalf("code=%d typed=%q stdout=%q stderr=%q", code, typed, stdout.String(), stderr.String())
	}
}

func TestVaultChangePayloadEncodesBothByteValuesAndIsErasedAfterDo(t *testing.T) {
	current := []byte("current \"master\" password")
	next := []byte("next\\master\npassword\u2028separator\u2029")
	payload, err := vaultChangePayload(current, next)
	if err != nil {
		t.Fatal(err)
	}
	if cap(payload) != len(payload) {
		t.Fatalf("change payload capacity=%d length=%d; exact allocation is required for zeroing", cap(payload), len(payload))
	}
	transport := &requestInspectingTransport{status: http.StatusNoContent}
	found := testHandoff("http://127.0.0.1:42803")
	response, err := sendVaultPOST(context.Background(), &http.Client{Transport: transport}, found, httpserver.VaultChangePath, payload)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()

	var decoded struct {
		Current string `json:"current"`
		Next    string `json:"next"`
	}
	if err := json.Unmarshal(transport.payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Current != "current \"master\" password" || decoded.Next != "next\\master\npassword\u2028separator\u2029" {
		t.Fatalf("decoded payload = %#v", decoded)
	}
	if !allZero(payload) {
		t.Fatalf("change JSON payload was not erased: %q", payload)
	}
}

func TestVaultPayloadEncoderEnforcesTheServerFourKiBBoundary(t *testing.T) {
	const passphraseEnvelope = len(`{"passphrase":""}`)
	exact := bytes.Repeat([]byte{'a'}, (4<<10)-passphraseEnvelope)
	payload, err := vaultPassphrasePayload(exact)
	if err != nil || len(payload) != 4<<10 {
		t.Fatalf("exact payload length=%d error=%v, want 4096 and nil", len(payload), err)
	}
	if cap(payload) != len(payload) {
		t.Fatalf("payload capacity=%d length=%d; exact allocation is required for zeroing", cap(payload), len(payload))
	}
	zeroBytes(payload)

	over := append(bytes.Clone(exact), 'a')
	payload, err = vaultPassphrasePayload(over)
	if payload != nil || !errors.Is(err, errVaultRequestTooLarge) {
		t.Fatalf("oversized payload=%v error=%v, want nil and too-large", payload, err)
	}

	// 入力バイト数が上限内でもエスケープ後に超過しうるため、最終的な上限は
	// エンコード済みペイロードで判定する。
	payload, err = vaultPassphrasePayload(bytes.Repeat([]byte{0}, 700))
	if payload != nil || !errors.Is(err, errVaultRequestTooLarge) {
		t.Fatalf("escaped oversized payload=%v error=%v", payload, err)
	}
}

func TestRunVaultCreateErasesTerminalBuffersAndSendsAuthenticatedJSON(t *testing.T) {
	first := []byte(vaultPasswordCanary)
	confirmation := []byte(vaultPasswordCanary)
	var posts int
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodGet:
			_, _ = io.WriteString(response, vaultStatusBody(handoff.OwnerEngine, false, false))
		case http.MethodPost:
			posts++
			if request.URL.Path != httpserver.VaultCreatePath || request.Header.Get(handoff.HeaderName) != "the secret" ||
				request.Header.Get("Content-Type") != "application/json" {
				response.WriteHeader(http.StatusUnauthorized)
				return
			}
			var body struct {
				Passphrase string `json:"passphrase"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil || body.Passphrase != vaultPasswordCanary {
				response.WriteHeader(http.StatusBadRequest)
				return
			}
			response.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()
	stateDir := t.TempDir()
	writeVaultTestHandoff(t, stateDir, server.URL, handoff.OwnerEngine)

	terminal := &fakePasswordTerminal{terminal: true, answers: [][]byte{first, confirmation}}
	var stdout, stderr strings.Builder
	code := runVault(context.Background(), "create", stateDir, server.Client(), vaultTestInput(t), &stdout, &stderr, terminal)
	if code != 0 || posts != 1 {
		t.Fatalf("code=%d posts=%d stdout=%q stderr=%q", code, posts, stdout.String(), stderr.String())
	}
	if !allZero(first) || !allZero(confirmation) {
		t.Fatalf("terminal buffers were not erased: first=%q confirmation=%q", first, confirmation)
	}
	if strings.Contains(stdout.String()+stderr.String(), vaultPasswordCanary) {
		t.Fatal("output leaked a password")
	}
}

func TestRunVaultRejectsInvalidUTF8WithoutSendingASecret(t *testing.T) {
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost {
			posts++
		}
		_, _ = io.WriteString(response, vaultStatusBody(handoff.OwnerEngine, false, false))
	}))
	defer server.Close()
	stateDir := t.TempDir()
	writeVaultTestHandoff(t, stateDir, server.URL, handoff.OwnerEngine)
	invalid := []byte{0xff, 0xfe}
	confirmation := []byte{0xff, 0xfe}

	var stdout, stderr strings.Builder
	code := runVault(context.Background(), "create", stateDir, server.Client(), vaultTestInput(t), &stdout, &stderr,
		&fakePasswordTerminal{terminal: true, answers: [][]byte{invalid, confirmation}})
	if code != 1 || posts != 0 || !allZero(invalid) || !allZero(confirmation) {
		t.Fatalf("code=%d posts=%d invalid=%v confirmation=%v stderr=%q", code, posts, invalid, confirmation, stderr.String())
	}
}

func TestRunVaultCancellationReturns130WithoutARequest(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	stateDir := t.TempDir()
	writeVaultTestHandoff(t, stateDir, server.URL, handoff.OwnerEngine)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var stdout, stderr strings.Builder
	code := runVault(ctx, "unlock", stateDir, server.Client(), vaultTestInput(t), &stdout, &stderr,
		&fakePasswordTerminal{terminal: true, answers: [][]byte{[]byte(vaultPasswordCanary)}})
	if code != 130 || requests != 0 {
		t.Fatalf("code=%d requests=%d stdout=%q stderr=%q", code, requests, stdout.String(), stderr.String())
	}
}

func TestRunVaultReadCancellationSendsNoMutation(t *testing.T) {
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost {
			posts++
		}
		_, _ = io.WriteString(response, vaultStatusBody(handoff.OwnerEngine, true, false))
	}))
	defer server.Close()
	stateDir := t.TempDir()
	writeVaultTestHandoff(t, stateDir, server.URL, handoff.OwnerEngine)

	var stdout, stderr strings.Builder
	code := runVault(context.Background(), "unlock", stateDir, server.Client(), vaultTestInput(t), &stdout, &stderr,
		&fakePasswordTerminal{terminal: true, errors: []error{context.Canceled}})
	if code != 130 || posts != 0 {
		t.Fatalf("code=%d posts=%d stdout=%q stderr=%q", code, posts, stdout.String(), stderr.String())
	}
}

func TestRunVaultErasesPartialPasswordReturnedWithReadError(t *testing.T) {
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost {
			posts++
		}
		_, _ = io.WriteString(response, vaultStatusBody(handoff.OwnerEngine, true, false))
	}))
	defer server.Close()
	stateDir := t.TempDir()
	writeVaultTestHandoff(t, stateDir, server.URL, handoff.OwnerEngine)
	typed := []byte(vaultPasswordCanary)

	var stdout, stderr strings.Builder
	code := runVault(context.Background(), "unlock", stateDir, server.Client(), vaultTestInput(t), &stdout, &stderr,
		&fakePasswordTerminal{
			terminal: true,
			answers:  [][]byte{typed},
			errors:   []error{vaultTransportError{message: []byte("read reflected " + vaultPasswordCanary)}},
		})
	if code != 1 || posts != 0 || !allZero(typed) {
		t.Fatalf("code=%d posts=%d typed=%q stdout=%q stderr=%q", code, posts, typed, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String()+stderr.String(), vaultPasswordCanary) {
		t.Fatal("password read error leaked a partial password")
	}
}

func TestRunVaultStopsPromptingWhenContextIsCanceledAfterARead(t *testing.T) {
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost {
			posts++
		}
		_, _ = io.WriteString(response, vaultStatusBody(handoff.OwnerEngine, false, false))
	}))
	defer server.Close()
	stateDir := t.TempDir()
	writeVaultTestHandoff(t, stateDir, server.URL, handoff.OwnerEngine)
	ctx, cancel := context.WithCancel(context.Background())
	typed := []byte(vaultPasswordCanary)
	terminal := &fakePasswordTerminal{
		terminal: true,
		answers:  [][]byte{typed},
		afterRead: func(index int) {
			if index == 0 {
				cancel()
			}
		},
	}

	var stdout, stderr strings.Builder
	code := runVault(ctx, "create", stateDir, server.Client(), vaultTestInput(t), &stdout, &stderr, terminal)
	if code != 130 || terminal.reads != 1 || posts != 0 {
		t.Fatalf("code=%d reads=%d posts=%d stdout=%q stderr=%q", code, terminal.reads, posts, stdout.String(), stderr.String())
	}
	if !allZero(typed) {
		t.Fatal("typed password was not erased after cancellation")
	}
}

type cancelingStatusBody struct {
	reader io.Reader
	cancel context.CancelFunc
}

func (b *cancelingStatusBody) Read(destination []byte) (int, error) {
	written, err := b.reader.Read(destination)
	if errors.Is(err, io.EOF) {
		b.cancel()
	}
	return written, err
}

func (*cancelingStatusBody) Close() error { return nil }

type cancelingStatusTransport struct {
	body     string
	cancel   context.CancelFunc
	requests int
}

func (t *cancelingStatusTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	t.requests++
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       &cancelingStatusBody{reader: strings.NewReader(t.body), cancel: t.cancel},
		Request:    request,
	}, nil
}

func TestRunVaultDoesNotPromptAfterPreflightCancellation(t *testing.T) {
	for _, test := range []struct {
		action   string
		vault    bool
		unlocked bool
	}{
		{action: "create", vault: false, unlocked: false},
		{action: "unlock", vault: true, unlocked: false},
		{action: "change-password", vault: true, unlocked: true},
	} {
		t.Run(test.action, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			transport := &cancelingStatusTransport{
				body:   vaultStatusBody(handoff.OwnerEngine, test.vault, test.unlocked),
				cancel: cancel,
			}
			stateDir := t.TempDir()
			writeVaultTestHandoff(t, stateDir, "http://127.0.0.1:42808", handoff.OwnerEngine)
			terminal := &fakePasswordTerminal{terminal: true}

			var stdout, stderr strings.Builder
			code := runVault(ctx, test.action, stateDir, &http.Client{Transport: transport}, vaultTestInput(t),
				&stdout, &stderr, terminal)
			if code != 130 || terminal.reads != 0 || transport.requests != 1 {
				t.Fatalf("code=%d reads=%d requests=%d stdout=%q stderr=%q",
					code, terminal.reads, transport.requests, stdout.String(), stderr.String())
			}
		})
	}
}

func TestRunVaultRejectsUnknownLiveOwnerWithoutHumanOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_, _ = io.WriteString(response, vaultStatusBody(handoff.Owner("unknown"), true, false))
	}))
	defer server.Close()
	stateDir := t.TempDir()
	writeVaultTestHandoff(t, stateDir, server.URL, handoff.OwnerEngine)

	var stdout, stderr strings.Builder
	code := runVault(context.Background(), "status", stateDir, server.Client(), vaultTestInput(t), &stdout, &stderr,
		&fakePasswordTerminal{terminal: false})
	if code != 1 || stdout.String() != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunVaultRejectsMalformedOrIncompatibleStatusBeforeHumanOutput(t *testing.T) {
	valid := `{"owner":"headless","version":"v4-test","protocolVersion":1,"vault":true,"unlocked":false,"sessions":2}`
	for _, test := range []struct {
		name string
		body string
	}{
		{name: "unknown field", body: strings.TrimSuffix(valid, "}") + `,"extra":true}`},
		{name: "trailing value", body: valid + ` {}`},
		{name: "missing sessions", body: `{"owner":"headless","version":"v4-test","protocolVersion":1,"vault":true,"unlocked":false}`},
		{name: "negative sessions", body: `{"owner":"headless","version":"v4-test","protocolVersion":1,"vault":true,"unlocked":false,"sessions":-1}`},
		{name: "impossible state", body: `{"owner":"headless","version":"v4-test","protocolVersion":1,"vault":false,"unlocked":true,"sessions":0}`},
		{name: "version mismatch", body: `{"owner":"headless","version":"another","protocolVersion":1,"vault":true,"unlocked":false,"sessions":0}`},
		{name: "protocol mismatch", body: `{"owner":"headless","version":"v4-test","protocolVersion":2,"vault":true,"unlocked":false,"sessions":0}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				_, _ = io.WriteString(response, test.body)
			}))
			defer server.Close()
			stateDir := t.TempDir()
			writeVaultTestHandoff(t, stateDir, server.URL, handoff.OwnerEngine)

			var stdout, stderr strings.Builder
			code := runVault(context.Background(), "status", stateDir, server.Client(), vaultTestInput(t), &stdout, &stderr,
				&fakePasswordTerminal{terminal: false})
			if code != 1 || stdout.String() != "" {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
		})
	}
}

type closingBody struct {
	reader io.Reader
	closed bool
}

func (b *closingBody) Read(buffer []byte) (int, error) { return b.reader.Read(buffer) }
func (b *closingBody) Close() error                    { b.closed = true; return nil }

type destinationCapturingBody struct {
	contents    []byte
	destination []byte
	read        bool
	closed      bool
}

func (b *destinationCapturingBody) Read(destination []byte) (int, error) {
	if b.read {
		return 0, io.EOF
	}
	b.read = true
	written := copy(destination, b.contents)
	b.destination = destination[:written]
	return written, nil
}

func (b *destinationCapturingBody) Close() error { b.closed = true; return nil }

func TestDiscardVaultResponseErasesTheReadBuffer(t *testing.T) {
	body := &destinationCapturingBody{contents: []byte("server reflected " + vaultPasswordCanary)}
	response := &http.Response{StatusCode: http.StatusInternalServerError, Body: body, Header: make(http.Header)}
	discardAndCloseVaultResponse(response)
	if !body.closed {
		t.Fatal("discarded response body was not closed")
	}
	if len(body.destination) == 0 || !allZero(body.destination) {
		t.Fatalf("discarded response buffer was not erased: %q", body.destination)
	}
}

type staticResponseTransport struct {
	status int
	body   *closingBody
}

func (t staticResponseTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: t.status, Header: make(http.Header), Body: t.body, Request: request}, nil
}

func TestRunVaultBoundsAndClosesEveryResponseBody(t *testing.T) {
	for _, test := range []struct {
		name   string
		action string
		status int
	}{
		{name: "status", action: "status", status: http.StatusOK},
		{name: "lock", action: "lock", status: http.StatusNoContent},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := &closingBody{reader: strings.NewReader(strings.Repeat("x", 70<<10))}
			client := &http.Client{Transport: staticResponseTransport{status: test.status, body: body}}
			stateDir := t.TempDir()
			writeVaultTestHandoff(t, stateDir, "http://127.0.0.1:42801", handoff.OwnerEngine)
			var stdout, stderr strings.Builder
			code := runVault(context.Background(), test.action, stateDir, client, vaultTestInput(t), &stdout, &stderr,
				&fakePasswordTerminal{terminal: false})
			if code != 1 || !body.closed {
				t.Fatalf("code=%d closed=%v stdout=%q stderr=%q", code, body.closed, stdout.String(), stderr.String())
			}
		})
	}
}

type sequencedResponseTransport struct {
	responses []*http.Response
	requests  int
}

func (t *sequencedResponseTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if t.requests >= len(t.responses) {
		return nil, errors.New("unexpected request")
	}
	response := t.responses[t.requests]
	t.requests++
	response.Request = request
	return response, nil
}

func TestRunVaultBoundsAndClosesErrorResponseBodies(t *testing.T) {
	for _, test := range []struct {
		name       string
		status     int
		body       string
		wantPhrase string
	}{
		{name: "ordinary error", status: http.StatusInternalServerError, body: "failed", wantPhrase: "operation failed"},
		{name: "unexpected response", status: http.StatusMultiStatus, body: strings.Repeat("x", 70<<10), wantPhrase: "invalid vault response"},
	} {
		t.Run(test.name, func(t *testing.T) {
			statusBody := &closingBody{reader: strings.NewReader(vaultStatusBody(handoff.OwnerEngine, true, true))}
			mutationBody := &closingBody{reader: strings.NewReader(test.body)}
			transport := &sequencedResponseTransport{responses: []*http.Response{
				{StatusCode: http.StatusOK, Header: make(http.Header), Body: statusBody},
				{StatusCode: test.status, Header: make(http.Header), Body: mutationBody},
			}}
			client := &http.Client{Transport: transport}
			stateDir := t.TempDir()
			writeVaultTestHandoff(t, stateDir, "http://127.0.0.1:42804", handoff.OwnerEngine)
			current := []byte("current password")
			next := []byte(vaultPasswordCanary)
			confirmation := []byte(vaultPasswordCanary)

			var stdout, stderr strings.Builder
			code := runVault(context.Background(), "change-password", stateDir, client, vaultTestInput(t), &stdout, &stderr,
				&fakePasswordTerminal{terminal: true, answers: [][]byte{current, next, confirmation}})
			if code != 1 || transport.requests != 2 || !statusBody.closed || !mutationBody.closed {
				t.Fatalf("code=%d requests=%d statusClosed=%v mutationClosed=%v stdout=%q stderr=%q",
					code, transport.requests, statusBody.closed, mutationBody.closed, stdout.String(), stderr.String())
			}
			if !strings.Contains(stderr.String(), test.wantPhrase) {
				t.Fatalf("stderr=%q, want phrase %q", stderr.String(), test.wantPhrase)
			}
			if strings.Contains(stdout.String()+stderr.String(), vaultPasswordCanary) {
				t.Fatal("error/partial response leaked the password")
			}
		})
	}
}

func allZero(value []byte) bool {
	for _, item := range value {
		if item != 0 {
			return false
		}
	}
	return true
}
