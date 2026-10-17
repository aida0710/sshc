package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/labstack/echo/v5"

	"sshc/internal/terminal"
	"sshc/internal/testwait"
)

func TestProgramProgressReachesSessionListWithoutAnAttachedBrowser(t *testing.T) {
	process := newScriptedPTY()
	registry := &terminal.Registry{}
	_, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindShell,
		Open: func(context.Context, terminal.Size) (terminal.Process, error) { return process, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { registry.BeginShutdown(); _ = registry.Wait() })
	engine := echo.New()
	registerTerminalRoutes(engine, TerminalHandlers{Registry: registry})
	steps := []struct {
		name, output, progress string
	}{
		{"normal", "\x1b]9;4;1;40\a", `{"state":"normal","percent":40}`},
		{"error keeps percent", "\x1b]9;4;2\a", `{"state":"error","percent":40}`},
		{"paused keeps percent", "\x1b]9;4;4\a", `{"state":"paused","percent":40}`},
		{"indeterminate", "\x1b]9;4;3\a", `{"state":"indeterminate"}`},
		{"clear", "\x1b]9;4;0\a", ""},
		{"zero percent", "\x1b]9;4;1;0\a", `{"state":"normal","percent":0}`},
		{"command finishes", "\x1b]133;D;0\a", ""},
	}
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			var want any
			if step.progress != "" {
				if err := json.Unmarshal([]byte(step.progress), &want); err != nil {
					t.Fatal(err)
				}
			}
			process.feed(step.output)
			testwait.Until(t, func() bool {
				response := httptest.NewRecorder()
				engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/terminal/sessions", nil))
				body := response.Body.String()
				if response.Code != http.StatusOK {
					t.Fatalf("list = %d: %s", response.Code, body)
				}
				var listed struct {
					Sessions []struct {
						ProgramProgress     json.RawMessage `json:"programProgress"`
						NotificationVersion int             `json:"notificationVersion"`
					}
				}
				if err := json.Unmarshal([]byte(body), &listed); err != nil {
					t.Fatal(err)
				}
				if len(listed.Sessions) != 1 {
					t.Fatalf("sessions = %s", body)
				}
				session := listed.Sessions[0]
				if session.NotificationVersion != 0 {
					t.Fatal("program progress became a notification")
				}
				if len(session.ProgramProgress) == 0 || step.progress == "" {
					return len(session.ProgramProgress) == 0 && step.progress == ""
				}
				var got any
				if err := json.Unmarshal(session.ProgramProgress, &got); err != nil {
					t.Fatal(err)
				}
				return reflect.DeepEqual(got, want)
			})
		})
	}
}
