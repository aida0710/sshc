package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"sshc/internal/terminal"
	"sshc/internal/testwait"
)

func TestProgramStatusReachesSessionListWithOnlyWhatTheProgramSaid(t *testing.T) {
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
	listedStatus := func() json.RawMessage {
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/terminal/sessions", nil))
		var listed struct {
			Sessions []struct {
				ProgramStatus json.RawMessage `json:"programStatus"`
			}
		}
		if err := json.Unmarshal(response.Body.Bytes(), &listed); err != nil || len(listed.Sessions) != 1 {
			t.Fatalf("list = %d: %s", response.Code, response.Body)
		}
		return listed.Sessions[0].ProgramStatus
	}

	// "Overwrite config.toml?" in base64.
	process.feed("\x1b]7501;state=blocked:kind=question:app=make:msg=T3ZlcndyaXRlIGNvbmZpZy50b21sPw==\x1b\\")
	want := `{"app":"make","kind":"question","message":"Overwrite config.toml?","state":"blocked","version":1}`
	testwait.Until(t, func() bool { return string(listedStatus()) == want })

	process.feed("\x1b]7501;state=working\x1b\\")
	testwait.Until(t, func() bool { return string(listedStatus()) == `{"state":"working","version":2}` })

	process.feed("\x1b]7501;state=clear\x1b\\")
	testwait.Until(t, func() bool { return len(listedStatus()) == 0 })
}
