package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/labstack/echo/v5"

	"sshc/internal/session"
	"sshc/internal/snippets"
	"sshc/internal/terminal"
)

type commandRepository struct {
	mutex   sync.Mutex
	library snippets.Library
}

func (r *commandRepository) Load() (snippets.Library, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.library, nil
}

func (r *commandRepository) Save(library snippets.Library) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.library = library
	return nil
}

func newTerminalCommandServer(t *testing.T) (*echo.Echo, session.Credentials, *terminal.Registry, *scriptedPTY, *snippets.Service) {
	t.Helper()
	manager, bootstrap, err := session.NewManager(bytes.NewReader(bytes.Repeat([]byte{0x4d}, 8192)))
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := manager.Bootstrap(bootstrap)
	if err != nil {
		t.Fatal(err)
	}
	process := newScriptedPTY()
	registry := &terminal.Registry{Limits: func() terminal.Limits {
		return terminal.Limits{MaxSessions: 4, Scrollback: 1 << 12}
	}}
	opened, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "production", Title: "Production shell",
		Open: func(context.Context, terminal.Size) (terminal.Process, error) { return process, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if opened.ID() == "" {
		t.Fatal("empty terminal session ID")
	}
	service := snippets.NewService(snippets.Options{Repository: &commandRepository{}})
	actions := ActionHandlers{Sessions: manager}
	engine := echo.New()
	engine.Use((Security{
		ExpectedHost: keyTestHost, ExpectedOrigin: "http://" + keyTestHost,
		Sessions: manager, Unlocked: alwaysUnlocked,
	}).Middleware)
	registerTerminalRoutes(engine, TerminalHandlers{
		Registry: registry, Tickets: &terminal.Tickets{}, Snippets: service, Actions: actions,
		ExpectedOrigin: "http://" + keyTestHost,
	})
	t.Cleanup(func() {
		registry.BeginShutdown()
		_ = registry.Wait()
	})
	return engine, credentials, registry, process, service
}

func TestTerminalCommandPreviewAndDispatchUseTheExistingPTY(t *testing.T) {
	engine, credentials, registry, process, _ := newTerminalCommandServer(t)
	views := registry.Sessions()
	request := terminalCommandPreviewRequest{
		Command: stringPointer("pwd"), Inputs: map[string]string{},
		Targets: []terminalCommandTargetRequest{{TargetId: "pane-a", SessionId: views[0].ID}},
	}
	previewResponse := sendKeyRequest(t, engine, credentials, http.MethodPost,
		"/api/v1/terminal/commands/preview", mustMarshal(t, request), "")
	if previewResponse.Code != http.StatusOK {
		t.Fatalf("preview = %d: %s", previewResponse.Code, previewResponse.Body.String())
	}
	var preview terminalCommandPreviewResponse
	if err := json.Unmarshal(previewResponse.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if len(preview.Targets) != 1 || preview.Targets[0].SessionId != views[0].ID || preview.Targets[0].Command != "pwd" {
		t.Fatalf("preview = %#v", preview)
	}
	dispatch := terminalCommandDispatchRequest{
		Command: request.Command, Inputs: request.Inputs, Targets: request.Targets, Evidence: preview.Evidence,
	}
	dispatched := sendKeyRequest(t, engine, credentials, http.MethodPost,
		"/api/v1/terminal/commands", mustMarshal(t, dispatch), preview.ActionToken)
	if dispatched.Code != http.StatusOK {
		t.Fatalf("dispatch = %d: %s", dispatched.Code, dispatched.Body.String())
	}
	var result terminalCommandDispatchResponse
	if err := json.Unmarshal(dispatched.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 1 || result.Results[0].Status != terminalCommandDelivered {
		t.Fatalf("result = %#v", result)
	}
	if got := process.keystrokes(); got != "pwd\r" {
		t.Fatalf("existing PTY input = %q", got)
	}

	replayed := sendKeyRequest(t, engine, credentials, http.MethodPost,
		"/api/v1/terminal/commands", mustMarshal(t, dispatch), preview.ActionToken)
	if replayed.Code != http.StatusForbidden {
		t.Fatalf("replayed token = %d, want 403", replayed.Code)
	}
	if got := process.keystrokes(); got != "pwd\r" {
		t.Fatalf("replay wrote %q", got)
	}
}

func TestTerminalCommandRefusesChangedPreviewAndSecretSnippet(t *testing.T) {
	engine, credentials, registry, process, service := newTerminalCommandServer(t)
	sessionID := registry.Sessions()[0].ID
	request := terminalCommandPreviewRequest{
		Command: stringPointer("pwd"), Inputs: map[string]string{},
		Targets: []terminalCommandTargetRequest{{TargetId: "pane-a", SessionId: sessionID}},
	}
	response := sendKeyRequest(t, engine, credentials, http.MethodPost,
		"/api/v1/terminal/commands/preview", mustMarshal(t, request), "")
	var preview terminalCommandPreviewResponse
	if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	changed := terminalCommandDispatchRequest{
		Command: stringPointer("whoami"), Inputs: request.Inputs, Targets: request.Targets, Evidence: preview.Evidence,
	}
	rejected := sendKeyRequest(t, engine, credentials, http.MethodPost,
		"/api/v1/terminal/commands", mustMarshal(t, changed), preview.ActionToken)
	if rejected.Code != http.StatusConflict || problemCode(t, rejected.Body.Bytes()) != "terminal_command_preview_changed" {
		t.Fatalf("changed preview = %d: %s", rejected.Code, rejected.Body.String())
	}
	if got := process.keystrokes(); got != "" {
		t.Fatalf("changed preview wrote %q", got)
	}

	snippet, err := service.Create(snippets.Draft{
		Name: "Secret", Command: "deploy --token={{token}}",
		Variables: []snippets.Variable{{Name: "token", Type: snippets.VariableSecret, Required: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	secretRequest := terminalCommandPreviewRequest{
		SnippetId: stringPointer(snippet.ID), Inputs: map[string]string{"token": "top-secret"}, Targets: request.Targets,
	}
	secret := sendKeyRequest(t, engine, credentials, http.MethodPost,
		"/api/v1/terminal/commands/preview", mustMarshal(t, secretRequest), "")
	if secret.Code != http.StatusBadRequest || problemCode(t, secret.Body.Bytes()) != "terminal_command_secret_refused" {
		t.Fatalf("secret preview = %d: %s", secret.Code, secret.Body.String())
	}
	if strings.Contains(secret.Body.String(), "top-secret") || process.keystrokes() != "" {
		t.Fatal("secret terminal command escaped or was written")
	}
}
