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
	"strings"
	"testing"
	"unicode/utf8"

	"sshc/internal/api"
)

const (
	setupAccessCanary    = "ACCESS-ID-MUST-NOT-ECHO"
	setupSecretCanary    = "SECRET-KEY-MUST-NOT-ECHO"
	setupExistingCanary  = "EXISTING-SYNC-KEY-MUST-NOT-ECHO"
	setupGeneratedCanary = "GENERATED-SYNC-KEY-SHOWN-ONCE"
)

type setupPasswordTerminal struct {
	terminals map[uintptr]bool
	answers   [][]byte
	errors    []error
	reads     int
	issued    [][]byte
}

func (terminal *setupPasswordTerminal) IsTerminal(fd int) bool {
	return terminal.terminals[uintptr(fd)]
}

func (terminal *setupPasswordTerminal) ReadPassword(
	ctx context.Context, _ *os.File, prompt func() error,
) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := prompt(); err != nil {
		return nil, err
	}
	index := terminal.reads
	terminal.reads++
	if index < len(terminal.errors) && terminal.errors[index] != nil {
		return nil, terminal.errors[index]
	}
	if index >= len(terminal.answers) {
		return nil, errors.New("no scripted hidden answer")
	}
	answer := bytes.Clone(terminal.answers[index])
	terminal.issued = append(terminal.issued, answer)
	return answer, nil
}

type syncSetupServer struct {
	t                *testing.T
	base             *engineAPIScript
	checkStatus      int
	checkResponse    api.SyncSetupCheckResponse
	completeStatus   int
	completeResponse api.SyncSetupResponse
	checkBodies      [][]byte
	completeBodies   [][]byte
}

func (server *syncSetupServer) handler(response http.ResponseWriter, request *http.Request) {
	switch request.URL.Path {
	case "/api/v1/sync/setup/check":
		body, err := io.ReadAll(request.Body)
		if err != nil {
			server.t.Error(err)
		}
		server.checkBodies = append(server.checkBodies, body)
		status := server.checkStatus
		if status == 0 {
			status = http.StatusOK
		}
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(status)
		if status >= 200 && status < 300 {
			_ = json.NewEncoder(response).Encode(server.checkResponse)
		} else {
			_, _ = io.WriteString(response, `{"code":"bucket_refused","message":"`+setupSecretCanary+`"}`)
		}
	case "/api/v1/sync/setup":
		body, err := io.ReadAll(request.Body)
		if err != nil {
			server.t.Error(err)
		}
		server.completeBodies = append(server.completeBodies, body)
		status := server.completeStatus
		if status == 0 {
			status = http.StatusOK
		}
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(status)
		if status >= 200 && status < 300 {
			_ = json.NewEncoder(response).Encode(server.completeResponse)
		} else {
			_, _ = io.WriteString(response, `{"code":"sync_setup_target_changed","message":"`+setupSecretCanary+`"}`)
		}
	default:
		server.base.handler(response, request)
	}
}

func newSyncSetupServer(t *testing.T, state api.SyncSetupTargetState) (*syncSetupServer, *httptest.Server, string) {
	t.Helper()
	etag := "etag-from-check"
	check := api.SyncSetupCheckResponse{
		CheckedAt: "2026-08-29T10:00:00Z", State: state, HistoryPresent: true, Etag: &etag,
	}
	if state == api.Empty {
		check.HistoryPresent = false
		check.Etag = nil
	}
	generated := setupGeneratedCanary
	result := api.SyncSetupResponse{Status: api.SyncStatus{
		Configured: true, Endpoint: "https://objects.example.test", Bucket: "ssh-config",
		Direction: api.SyncDirectionBoth, KeyConfigured: true, Auto: api.AutoSync{Phase: api.AutoSyncPhaseIdle},
	}}
	if state == api.Empty {
		result.GeneratedKey = &generated
	}
	currentBody, err := json.Marshal(api.SyncStatus{
		Direction: api.SyncDirectionBoth, Auto: api.AutoSync{Phase: api.AutoSyncPhaseIdle},
	})
	if err != nil {
		t.Fatal(err)
	}
	script := &syncSetupServer{
		t: t,
		base: &engineAPIScript{
			t: t, statusBody: validEngineStatus(), syncBody: string(currentBody),
		},
		checkResponse: check, completeResponse: result,
	}
	httpServer := httptest.NewServer(http.HandlerFunc(script.handler))
	stateDir := t.TempDir()
	writeTestHandoff(t, stateDir, httpServer.URL)
	return script, httpServer, stateDir
}

type setupRunResult struct {
	code   int
	stdout string
	prompt string
}

func setSetupCurrentStatus(t *testing.T, server *syncSetupServer, status api.SyncStatus) {
	t.Helper()
	body, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	server.base.syncBody = string(body)
}

func runSetupFixture(
	t *testing.T,
	stateDir string,
	client *http.Client,
	visible string,
	terminal *setupPasswordTerminal,
) setupRunResult {
	t.Helper()
	input, err := os.CreateTemp(t.TempDir(), "setup-input-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = input.Close() })
	if _, err := input.WriteString(visible); err != nil {
		t.Fatal(err)
	}
	if _, err := input.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	prompt, err := os.CreateTemp(t.TempDir(), "setup-prompt-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = prompt.Close() })
	if terminal.terminals == nil {
		terminal.terminals = map[uintptr]bool{}
	}
	terminal.terminals[input.Fd()] = true
	terminal.terminals[prompt.Fd()] = true

	var stdout strings.Builder
	code := runSync(context.Background(), syncInvocation{Action: syncSetup}, stateDir,
		client, input, &stdout, prompt, terminal)
	if _, err := prompt.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	promptBody, err := io.ReadAll(prompt)
	if err != nil {
		t.Fatal(err)
	}
	return setupRunResult{code: code, stdout: stdout.String(), prompt: string(promptBody)}
}

func standardVisibleSetup() string {
	return strings.Join([]string{
		"https://objects.example.test", "ssh-config", "team/hosts", "ap-northeast-1", "both", "",
	}, "\n")
}

func standardHiddenSetup(existing bool) [][]byte {
	answers := [][]byte{[]byte(setupAccessCanary), []byte(setupSecretCanary)}
	if existing {
		answers = append(answers, []byte(setupExistingCanary))
	}
	return answers
}

func TestSyncSetupRequiresTTYBeforeContactingTheEngine(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	stateDir := t.TempDir()
	writeTestHandoff(t, stateDir, server.URL)

	input, err := os.CreateTemp(t.TempDir(), "not-terminal-")
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	var stdout, stderr strings.Builder
	terminal := &setupPasswordTerminal{terminals: map[uintptr]bool{input.Fd(): false}}
	code := runSync(context.Background(), syncInvocation{Action: syncSetup}, stateDir,
		server.Client(), input, &stdout, &stderr, terminal)
	if code != 1 || requests != 0 || !strings.Contains(stderr.String(), "interactive terminal") {
		t.Fatalf("code=%d requests=%d stdout=%q stderr=%q", code, requests, stdout.String(), stderr.String())
	}

	// Even with terminal stdin, a redirected prompt stream cannot safely show a
	// generated key exactly once.
	terminal.terminals[input.Fd()] = true
	stdout.Reset()
	stderr.Reset()
	code = runSync(context.Background(), syncInvocation{Action: syncSetup}, stateDir,
		server.Client(), input, &stdout, &stderr, terminal)
	if code != 1 || requests != 0 {
		t.Fatalf("redirected prompt: code=%d requests=%d", code, requests)
	}
}

func TestSyncSetupPromptsWithoutEchoingSecrets(t *testing.T) {
	script, server, stateDir := newSyncSetupServer(t, api.Existing)
	defer server.Close()
	terminal := &setupPasswordTerminal{answers: standardHiddenSetup(true)}
	result := runSetupFixture(t, stateDir, server.Client(), standardVisibleSetup(), terminal)
	if result.code != 0 {
		t.Fatalf("code=%d stdout=%q prompt=%q", result.code, result.stdout, result.prompt)
	}
	for _, prompt := range []string{
		"Endpoint [https://]:", "Bucket []:", "Path []:", "Region [auto]:", "Direction [both] (both/push/pull):",
		"Access key ID:", "Secret access key:", "Sync key:",
	} {
		if !strings.Contains(result.prompt, prompt) {
			t.Errorf("prompt omitted %q: %q", prompt, result.prompt)
		}
	}
	for _, secret := range []string{setupAccessCanary, setupSecretCanary, setupExistingCanary} {
		if strings.Contains(result.prompt+result.stdout, secret) {
			t.Fatalf("setup output echoed %q", secret)
		}
	}
	for _, answer := range standardHiddenSetup(true) {
		if !strings.Contains(result.prompt, strings.Repeat("*", utf8.RuneCount(answer))) {
			t.Errorf("setup prompt did not confirm hidden input length %d: %q", utf8.RuneCount(answer), result.prompt)
		}
	}
	if len(script.checkBodies) != 1 || len(script.completeBodies) != 1 {
		t.Fatalf("check=%d complete=%d", len(script.checkBodies), len(script.completeBodies))
	}
	for index, secret := range terminal.issued {
		if !allZero(secret) {
			t.Errorf("hidden input %d was not erased: %q", index, secret)
		}
	}
}

func TestSyncSetupUsesExistingDefaultsAndKeepsHiddenValuesOnBlankInput(t *testing.T) {
	script, server, stateDir := newSyncSetupServer(t, api.Existing)
	defer server.Close()
	path := "team/hosts"
	region := "ap-northeast-1"
	suffix := "AMPLE"
	setSetupCurrentStatus(t, script, api.SyncStatus{
		Configured: true, Endpoint: "https://objects.example.test", Bucket: "ssh-config",
		Path: &path, Region: &region, Direction: api.SyncDirectionPull,
		KeyConfigured: true, AccessKeySuffix: &suffix, Auto: api.AutoSync{Phase: api.AutoSyncPhaseIdle},
	})
	result := runSetupFixture(t, stateDir, server.Client(), "\n\n\n\n\n",
		&setupPasswordTerminal{answers: [][]byte{{}, {}, {}}})
	if result.code != 0 {
		t.Fatalf("code=%d stdout=%q prompt=%q", result.code, result.stdout, result.prompt)
	}
	for _, want := range []string{
		"Endpoint [https://objects.example.test]:", "Bucket [ssh-config]:", "Path [team/hosts]:",
		"Region [ap-northeast-1]:", "Direction [pull] (both/push/pull):",
		"Access key ID [*****AMPLE; Enter to keep]:",
		"Secret access key [configured; Enter to keep]:",
		"Sync key [configured; Enter to keep]:",
	} {
		if !strings.Contains(result.prompt, want) {
			t.Errorf("prompt omitted %q: %q", want, result.prompt)
		}
	}
	if len(script.checkBodies) != 1 || len(script.completeBodies) != 1 {
		t.Fatalf("check=%d complete=%d", len(script.checkBodies), len(script.completeBodies))
	}
	var check api.SyncSetupCheckRequest
	if err := json.Unmarshal(script.checkBodies[0], &check); err != nil {
		t.Fatal(err)
	}
	if !check.ReuseCredentials || check.AccessKeyId != nil || check.SecretAccessKey != nil ||
		check.Endpoint != "https://objects.example.test" || check.Bucket != "ssh-config" {
		t.Fatalf("check request = %+v", check)
	}
	var complete api.SyncSetupRequest
	if err := json.Unmarshal(script.completeBodies[0], &complete); err != nil {
		t.Fatal(err)
	}
	if !complete.ReuseCredentials || !complete.ReuseKey || complete.AccessKeyId != nil ||
		complete.SecretAccessKey != nil || complete.Key != "" || complete.Direction != api.SyncDirectionPull {
		t.Fatalf("complete request = %+v", complete)
	}
}

func TestSyncSetupAcceptsWindowsCRLFWithoutSkippingPrompts(t *testing.T) {
	script, server, stateDir := newSyncSetupServer(t, api.Existing)
	defer server.Close()
	visible := strings.ReplaceAll(standardVisibleSetup(), "\n", "\r\n")
	result := runSetupFixture(t, stateDir, server.Client(), visible,
		&setupPasswordTerminal{answers: standardHiddenSetup(true)})
	if result.code != 0 || len(script.checkBodies) != 1 {
		t.Fatalf("code=%d checks=%d stdout=%q prompt=%q",
			result.code, len(script.checkBodies), result.stdout, result.prompt)
	}
	var check api.SyncSetupCheckRequest
	if err := json.Unmarshal(script.checkBodies[0], &check); err != nil {
		t.Fatal(err)
	}
	if check.Endpoint != "https://objects.example.test" || check.Bucket != "ssh-config" ||
		check.Path == nil || *check.Path != "team/hosts" || check.Region == nil ||
		*check.Region != "ap-northeast-1" {
		t.Fatalf("CRLF input shifted setup answers: %+v", check)
	}
}

func TestSyncSetupChecksBeforeSavingExistingTarget(t *testing.T) {
	script, server, stateDir := newSyncSetupServer(t, api.Existing)
	defer server.Close()
	result := runSetupFixture(t, stateDir, server.Client(), standardVisibleSetup(),
		&setupPasswordTerminal{answers: standardHiddenSetup(true)})
	if result.code != 0 || len(script.checkBodies) != 1 || len(script.completeBodies) != 1 {
		t.Fatalf("code=%d check=%d complete=%d stdout=%q prompt=%q",
			result.code, len(script.checkBodies), len(script.completeBodies), result.stdout, result.prompt)
	}
	var check api.SyncSetupCheckRequest
	if err := json.Unmarshal(script.checkBodies[0], &check); err != nil {
		t.Fatal(err)
	}
	if check.Endpoint != "https://objects.example.test" || check.Bucket != "ssh-config" ||
		check.Path == nil || *check.Path != "team/hosts" || check.Region == nil || *check.Region != "ap-northeast-1" ||
		check.AccessKeyId == nil || *check.AccessKeyId != setupAccessCanary ||
		check.SecretAccessKey == nil || *check.SecretAccessKey != setupSecretCanary || check.ReuseCredentials {
		t.Fatalf("check request = %+v", check)
	}
	var complete api.SyncSetupRequest
	if err := json.Unmarshal(script.completeBodies[0], &complete); err != nil {
		t.Fatal(err)
	}
	if complete.ExpectedState != api.Existing || complete.ExpectedETag == nil ||
		*complete.ExpectedETag != "etag-from-check" || !complete.HistoryPresent ||
		complete.Key != setupExistingCanary || complete.Direction != api.SyncDirectionBoth ||
		complete.ReuseCredentials || complete.ReuseKey {
		t.Fatalf("complete request did not preserve check evidence: %+v", complete)
	}
}

func TestSyncSetupEmptyPrintsGeneratedKeyOnceOnlyOnPromptTerminal(t *testing.T) {
	script, server, stateDir := newSyncSetupServer(t, api.Empty)
	defer server.Close()
	result := runSetupFixture(t, stateDir, server.Client(), standardVisibleSetup(),
		&setupPasswordTerminal{answers: standardHiddenSetup(false)})
	if result.code != 0 || len(script.completeBodies) != 1 {
		t.Fatalf("code=%d stdout=%q prompt=%q", result.code, result.stdout, result.prompt)
	}
	var complete api.SyncSetupRequest
	if err := json.Unmarshal(script.completeBodies[0], &complete); err != nil {
		t.Fatal(err)
	}
	if complete.Key != "" || complete.ExpectedState != api.Empty || complete.ExpectedETag != nil {
		t.Fatalf("empty-target complete = %+v", complete)
	}
	if strings.Count(result.prompt, setupGeneratedCanary) != 1 || strings.Contains(result.stdout, setupGeneratedCanary) {
		t.Fatalf("generated key placement: stdout=%q prompt=%q", result.stdout, result.prompt)
	}
}

func TestSyncSetupIncompleteAndCheckFailureNeverSave(t *testing.T) {
	t.Run("incomplete", func(t *testing.T) {
		script, server, stateDir := newSyncSetupServer(t, api.Incomplete)
		defer server.Close()
		result := runSetupFixture(t, stateDir, server.Client(), standardVisibleSetup(),
			&setupPasswordTerminal{answers: standardHiddenSetup(false)})
		if result.code != 1 || len(script.completeBodies) != 0 || strings.Contains(result.stdout, "configured") {
			t.Fatalf("code=%d complete=%d stdout=%q prompt=%q",
				result.code, len(script.completeBodies), result.stdout, result.prompt)
		}
	})

	t.Run("check error", func(t *testing.T) {
		script, server, stateDir := newSyncSetupServer(t, api.Empty)
		defer server.Close()
		script.checkStatus = http.StatusBadGateway
		result := runSetupFixture(t, stateDir, server.Client(), standardVisibleSetup(),
			&setupPasswordTerminal{answers: standardHiddenSetup(false)})
		if result.code != 1 || len(script.completeBodies) != 0 ||
			strings.Contains(result.stdout+result.prompt, setupSecretCanary) {
			t.Fatalf("code=%d complete=%d stdout=%q prompt=%q",
				result.code, len(script.completeBodies), result.stdout, result.prompt)
		}
	})
}

func TestSyncSetupTargetChangeDoesNotPrintSuccess(t *testing.T) {
	script, server, stateDir := newSyncSetupServer(t, api.Existing)
	defer server.Close()
	script.completeStatus = http.StatusConflict
	result := runSetupFixture(t, stateDir, server.Client(), standardVisibleSetup(),
		&setupPasswordTerminal{answers: standardHiddenSetup(true)})
	if result.code != 1 || len(script.completeBodies) != 1 || strings.Contains(result.stdout, "configured") ||
		strings.Contains(result.stdout+result.prompt, setupSecretCanary) {
		t.Fatalf("code=%d complete=%d stdout=%q prompt=%q",
			result.code, len(script.completeBodies), result.stdout, result.prompt)
	}
}

func TestSyncSetupCanceledDuringHiddenPrompts(t *testing.T) {
	for _, test := range []struct {
		name      string
		state     api.SyncSetupTargetState
		errors    []error
		answers   [][]byte
		wantCheck int
	}{
		{name: "access key", state: api.Empty, errors: []error{context.Canceled}},
		{name: "secret key", state: api.Empty, errors: []error{nil, context.Canceled}, answers: [][]byte{[]byte(setupAccessCanary)}},
		{name: "existing sync key", state: api.Existing, errors: []error{nil, nil, context.Canceled}, answers: standardHiddenSetup(false), wantCheck: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			script, server, stateDir := newSyncSetupServer(t, test.state)
			defer server.Close()
			result := runSetupFixture(t, stateDir, server.Client(), standardVisibleSetup(),
				&setupPasswordTerminal{answers: test.answers, errors: test.errors})
			if result.code != 130 || len(script.checkBodies) != test.wantCheck || len(script.completeBodies) != 0 {
				t.Fatalf("code=%d check=%d complete=%d stdout=%q prompt=%q",
					result.code, len(script.checkBodies), len(script.completeBodies), result.stdout, result.prompt)
			}
		})
	}
}

func TestSyncSetupCanceledAtEveryVisiblePrompt(t *testing.T) {
	answers := []string{
		"https://objects.example.test",
		"ssh-config",
		"team/hosts",
		"ap-northeast-1",
		"both",
	}
	for cancelAt := range answers {
		t.Run(answers[cancelAt], func(t *testing.T) {
			script, server, stateDir := newSyncSetupServer(t, api.Empty)
			defer server.Close()
			visible := strings.Join(answers[:cancelAt], "\n")
			if visible != "" {
				visible += "\n"
			}
			visible += string([]byte{0x03})
			terminal := &setupPasswordTerminal{answers: standardHiddenSetup(false)}
			result := runSetupFixture(t, stateDir, server.Client(), visible, terminal)
			if result.code != 130 || len(script.checkBodies) != 0 || len(script.completeBodies) != 0 || terminal.reads != 0 {
				t.Fatalf("code=%d check=%d complete=%d hiddenReads=%d stdout=%q prompt=%q",
					result.code, len(script.checkBodies), len(script.completeBodies), terminal.reads,
					result.stdout, result.prompt)
			}
		})
	}
}

func TestSyncSetupRejectsInvalidOrOversizedInputBeforeCheck(t *testing.T) {
	tests := []struct {
		name    string
		visible string
		answers [][]byte
	}{
		{"non HTTPS", "http://objects.example.test\nssh-config\n\nauto\nboth\n", standardHiddenSetup(false)},
		{"unsafe bucket", "https://objects.example.test\n../bucket\n\nauto\nboth\n", standardHiddenSetup(false)},
		{"unsafe path", "https://objects.example.test\nssh-config\nteam/../hosts\nauto\nboth\n", standardHiddenSetup(false)},
		{"bad direction", "https://objects.example.test\nssh-config\n\nauto\nsideways\n", standardHiddenSetup(false)},
		{"long line", strings.Repeat("x", 4097) + "\nssh-config\n\nauto\nboth\n", standardHiddenSetup(false)},
		{"long access", standardVisibleSetup(), [][]byte{bytes.Repeat([]byte{'a'}, 513), []byte(setupSecretCanary)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			script, server, stateDir := newSyncSetupServer(t, api.Empty)
			defer server.Close()
			result := runSetupFixture(t, stateDir, server.Client(), test.visible,
				&setupPasswordTerminal{answers: test.answers})
			if result.code != 1 || len(script.checkBodies) != 0 || len(script.completeBodies) != 0 {
				t.Fatalf("code=%d check=%d complete=%d stdout=%q prompt=%q",
					result.code, len(script.checkBodies), len(script.completeBodies), result.stdout, result.prompt)
			}
		})
	}
}
