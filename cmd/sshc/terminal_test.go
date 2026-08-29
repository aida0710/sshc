package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"sshc/internal/api"
	"sshc/internal/httpserver"
)

const terminalTestID = "0123456789abcdef0123456789abcdef"

func testTerminalEngine(t *testing.T, handler http.Handler) *engineAPI {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &engineAPI{
		origin: server.URL, cookie: http.Cookie{Name: "session", Value: "test"}, csrf: "csrf",
		client: server.Client(),
	}
}

func terminalTestSession(state string) api.TerminalSession {
	return api.TerminalSession{
		Id: terminalTestID, Kind: api.TerminalSessionKind("shell"), Title: "zsh",
		State: api.TerminalSessionState(state), StartedAt: "2026-08-30T00:00:00Z",
	}
}

func TestTerminalReadUsesTheResolvedIDAndBoundedCursor(t *testing.T) {
	engine := testTerminalEngine(t, http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/terminal/sessions":
			_ = json.NewEncoder(response).Encode(api.TerminalSessionList{Sessions: []api.TerminalSession{terminalTestSession("connected")}})
		case "/api/v1/terminal/sessions/" + terminalTestID + "/control":
			if request.URL.Query().Get("cursor") != "41" || request.URL.Query().Get("limit") != "1024" {
				t.Errorf("query = %q", request.URL.RawQuery)
			}
			_ = json.NewEncoder(response).Encode(map[string]any{
				"session": terminalTestSession("connected"), "generation": 3, "state": "connected",
				"cursor": map[string]any{"requested": 41, "start": 41, "next": 46, "end": 46, "truncated": false},
				"output": "hello",
			})
		default:
			http.NotFound(response, request)
		}
	}))
	result, err := executeTerminal(context.Background(), engine, terminalInvocation{
		Action: terminalRead, Selector: terminalTestID[:8], Cursor: 41, Limit: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	control := result.(terminalControlWire)
	if control.Output != "hello" || control.Generation != 3 || control.Cursor.Next != 46 {
		t.Fatalf("control = %#v", control)
	}
}

func TestTerminalSendUsesPreviewEvidenceAndOneTimeAction(t *testing.T) {
	var previews, dispatches atomic.Int32
	engine := testTerminalEngine(t, http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/terminal/sessions":
			_ = json.NewEncoder(response).Encode(api.TerminalSessionList{Sessions: []api.TerminalSession{terminalTestSession("connected")}})
		case "/api/v1/terminal/commands/preview":
			previews.Add(1)
			var body api.TerminalCommandPreviewRequest
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil || body.Command == nil || *body.Command != "uptime" || body.Submit == nil || !*body.Submit {
				t.Errorf("preview body = %#v, %v", body, err)
			}
			_ = json.NewEncoder(response).Encode(api.TerminalCommandPreview{
				Evidence: "generation-bound-evidence", ActionToken: "one-time-action", ActionExpiresAt: "later",
				Targets: []api.TerminalCommandPreviewTarget{{TargetId: "cli", SessionId: terminalTestID, Title: "zsh"}},
			})
		case "/api/v1/terminal/commands":
			dispatches.Add(1)
			if request.Header.Get(httpserver.ActionHeader) != "one-time-action" {
				t.Errorf("action header = %q", request.Header.Get(httpserver.ActionHeader))
			}
			var body api.TerminalCommandDispatchRequest
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil || body.Evidence != "generation-bound-evidence" {
				t.Errorf("dispatch body = %#v, %v", body, err)
			}
			_ = json.NewEncoder(response).Encode(api.TerminalCommandDispatchResponse{Results: []api.TerminalCommandResult{{
				TargetId: "cli", SessionId: terminalTestID, Title: "zsh", Status: api.TerminalCommandResultStatusDelivered,
			}}})
		default:
			http.NotFound(response, request)
		}
	}))
	result, err := executeTerminal(context.Background(), engine, terminalInvocation{
		Action: terminalSend, Selector: terminalTestID, Text: "uptime", Submit: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.(api.TerminalCommandResult).Status != api.TerminalCommandResultStatusDelivered || previews.Load() != 1 || dispatches.Load() != 1 {
		t.Fatalf("result = %#v, previews=%d dispatches=%d", result, previews.Load(), dispatches.Load())
	}
}

func TestTerminalWaitPollsOnlyExplicitLifecycleState(t *testing.T) {
	var reads atomic.Int32
	engine := testTerminalEngine(t, http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/terminal/sessions":
			_ = json.NewEncoder(response).Encode(api.TerminalSessionList{Sessions: []api.TerminalSession{terminalTestSession("connected")}})
		case "/api/v1/terminal/sessions/" + terminalTestID + "/control":
			state := "agent-working"
			if reads.Add(1) >= 2 {
				state = "agent-ready"
			}
			_ = json.NewEncoder(response).Encode(map[string]any{
				"session": terminalTestSession("connected"), "generation": 1, "state": state,
				"cursor": map[string]any{"requested": 0, "start": 0, "next": 0, "end": 0, "truncated": false}, "output": "",
			})
		default:
			http.NotFound(response, request)
		}
	}))
	result, err := executeTerminal(context.Background(), engine, terminalInvocation{
		Action: terminalWait, Selector: terminalTestID, WaitFor: "agent-ready", Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.(terminalControlWire).State != "agent-ready" || reads.Load() != 2 {
		t.Fatalf("result = %#v, reads=%d", result, reads.Load())
	}
}

func TestTerminalSelectorRejectsAmbiguousPrefixes(t *testing.T) {
	sessions := []api.TerminalSession{{Id: "01234567aaaaaaaa"}, {Id: "01234567bbbbbbbb"}}
	if _, err := resolveTerminalSession(sessions, "01234567"); !errors.Is(err, errTerminalSelectorAmbiguous) {
		t.Fatalf("resolve = %v", err)
	}
}

func TestTerminalControlCursorRequiresConsistentTruncationEvidence(t *testing.T) {
	tests := []struct {
		name                        string
		requested, start, next, end uint64
		truncated                   bool
		want                        bool
	}{
		{name: "exact range", requested: 7, start: 7, next: 11, end: 11, want: true},
		{name: "retained range", requested: 7, start: 9, next: 11, end: 13, truncated: true, want: true},
		{name: "false truncation claim", requested: 7, start: 7, next: 11, end: 11, truncated: true},
		{name: "unreported truncation", requested: 7, start: 9, next: 11, end: 11},
		{name: "past transcript end", requested: 7, start: 7, next: 12, end: 11},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := validTerminalControlCursor(
				test.requested, test.start, test.next, test.end, test.truncated, test.requested, 64,
			)
			if got != test.want {
				t.Fatalf("validTerminalControlCursor() = %v, want %v", got, test.want)
			}
		})
	}
}
