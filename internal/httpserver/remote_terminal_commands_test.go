package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"sshc/internal/remoteengine"
	"sshc/internal/session"
	"sshc/internal/snippets"
)

// relayToEngine は、実行先の役のエンジンへ、このマシンのエンジンが受け取った資格で要求を送る
// 偽物の RemoteEngineDirectory である。remoteengine.Manager が SSH の上で行うことを、同じ
// プロセスの中で行う。
type relayToEngine struct {
	fakeEngineDirectory
	t           *testing.T
	remote      *echo.Echo
	credentials session.Credentials
}

func (r *relayToEngine) CallTerminalAPI(_ context.Context, _ string, call remoteengine.TerminalCall) (remoteengine.TerminalAnswer, error) {
	recorder := sendKeyRequest(r.t, r.remote, r.credentials, call.Method, call.Path, call.Body, call.ActionToken)
	body, _ := io.ReadAll(recorder.Result().Body)
	return remoteengine.TerminalAnswer{Status: recorder.Code, Body: body}, nil
}

// localEngineForRunTarget は、このマシンのエンジン（実行先の受け口と、このマシンのスニペット）を立てる。
func localEngineForRunTarget(t *testing.T, directory RemoteEngineDirectory) (*echo.Echo, session.Credentials, *snippets.Service) {
	t.Helper()
	manager, bootstrap, err := session.NewManager(bytes.NewReader(bytes.Repeat([]byte{0x5a}, 8192)))
	if err != nil {
		t.Fatal(err)
	}
	credentials, _, err := manager.BootstrapForSession(bootstrap, "")
	if err != nil {
		t.Fatal(err)
	}
	service := snippets.NewService(snippets.Options{Repository: &commandRepository{}})
	engine := echo.New()
	engine.Use((Security{
		ExpectedHost: keyTestHost, ExpectedOrigin: "http://" + keyTestHost, Sessions: manager, Unlocked: alwaysUnlocked,
	}).Middleware)
	registerRemoteEngineRoutes(engine, RemoteEngineHandlers{
		Engines: directory, ExpectedOrigin: "http://" + keyTestHost, Snippets: service, Actions: ActionHandlers{Sessions: manager},
	})
	return engine, credentials, service
}

func TestARunTargetCommandIsExpandedHereShownRedactedAndSentOnceWithBothConfirmations(t *testing.T) {
	remote, remoteCredentials, registry, process, _ := newTerminalCommandServer(t)
	directory := &relayToEngine{t: t, remote: remote, credentials: remoteCredentials}
	directory.engines = []string{"r540"}
	local, credentials, service := localEngineForRunTarget(t, directory)
	snippet, err := service.Create(snippets.Draft{
		Name: "Deploy", Command: "deploy --token={{token}}",
		Variables: []snippets.Variable{{Name: "token", Type: snippets.VariableSecret, Required: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := terminalCommandPreviewRequest{
		SnippetId: stringPointer(snippet.ID), Inputs: map[string]string{"token": "top-secret"},
		Targets: []terminalCommandTargetRequest{{TargetId: "pane-a", SessionId: registry.Sessions()[0].ID}},
	}

	previewed := sendKeyRequest(t, local, credentials, http.MethodPost, "/api/v1/engines/r540/terminal/commands/preview", mustMarshal(t, request), "")
	if previewed.Code != http.StatusOK || strings.Contains(previewed.Body.String(), "top-secret") || !strings.Contains(previewed.Body.String(), "[secret]") {
		t.Fatalf("preview = %d: %s", previewed.Code, previewed.Body.String())
	}
	var preview terminalCommandPreviewResponse
	if err := json.Unmarshal(previewed.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if len(preview.Targets) != 1 || preview.Targets[0].Alias != "production" || preview.ActionToken == "" {
		t.Fatalf("preview = %#v", preview)
	}

	dispatch := terminalCommandDispatchRequest{
		SnippetId: request.SnippetId, Inputs: request.Inputs, Targets: request.Targets, Evidence: preview.Evidence,
	}
	sent := sendKeyRequest(t, local, credentials, http.MethodPost, "/api/v1/engines/r540/terminal/commands", mustMarshal(t, dispatch), preview.ActionToken)
	if sent.Code != http.StatusOK || strings.Contains(sent.Body.String(), "top-secret") {
		t.Fatalf("dispatch = %d: %s", sent.Code, sent.Body.String())
	}
	if got := process.keystrokes(); got != "deploy --token=top-secret\r" {
		t.Fatalf("run target's PTY received %q", got)
	}

	replayed := sendKeyRequest(t, local, credentials, http.MethodPost, "/api/v1/engines/r540/terminal/commands", mustMarshal(t, dispatch), preview.ActionToken)
	if replayed.Code != http.StatusForbidden {
		t.Fatalf("replayed token = %d: %s", replayed.Code, replayed.Body.String())
	}
	if got := process.keystrokes(); got != "deploy --token=top-secret\r" {
		t.Fatalf("replay wrote %q", got)
	}
}

func TestARunTargetCommandWhoseSnippetChangedAfterThePreviewIsNotSent(t *testing.T) {
	remote, remoteCredentials, registry, process, _ := newTerminalCommandServer(t)
	directory := &relayToEngine{t: t, remote: remote, credentials: remoteCredentials}
	directory.engines = []string{"r540"}
	local, credentials, _ := localEngineForRunTarget(t, directory)
	request := terminalCommandPreviewRequest{
		Command: stringPointer("pwd"), Inputs: map[string]string{},
		Targets: []terminalCommandTargetRequest{{TargetId: "pane-a", SessionId: registry.Sessions()[0].ID}},
	}
	previewed := sendKeyRequest(t, local, credentials, http.MethodPost, "/api/v1/engines/r540/terminal/commands/preview", mustMarshal(t, request), "")
	var preview terminalCommandPreviewResponse
	if err := json.Unmarshal(previewed.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	changed := terminalCommandDispatchRequest{
		Command: stringPointer("rm -rf ~"), Inputs: request.Inputs, Targets: request.Targets, Evidence: preview.Evidence,
	}
	rejected := sendKeyRequest(t, local, credentials, http.MethodPost, "/api/v1/engines/r540/terminal/commands", mustMarshal(t, changed), preview.ActionToken)
	if rejected.Code != http.StatusConflict || problemCode(t, rejected.Body.Bytes()) != "terminal_command_preview_changed" {
		t.Fatalf("changed = %d: %s", rejected.Code, rejected.Body.String())
	}
	if got := process.keystrokes(); got != "" {
		t.Fatalf("a changed command reached the run target: %q", got)
	}
}
