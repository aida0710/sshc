package httpserver

import (
	"context"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/remoteengine"
	"sshc/internal/session"
)

// fakeEngineDirectory は、実行先の一覧と、取り次ぎに渡された alias と接続先のパスを覚える。
type fakeEngineDirectory struct {
	fakeRemoteEngines
	engines    []string
	addErr     error
	forwarded  []string
	streamed   []string
	statusSeen remoteengine.Status
	statusErr  error
	controlled []string
	unlocked   []string
}

func (f *fakeEngineDirectory) Engines() ([]string, error) { return f.engines, nil }
func (f *fakeEngineDirectory) AddEngine(_ context.Context, alias string) error {
	if f.addErr != nil {
		return f.addErr
	}
	f.engines = append(f.engines, alias)
	return nil
}
func (f *fakeEngineDirectory) RemoveEngine(string) error { f.engines = nil; return nil }
func (f *fakeEngineDirectory) EngineStatus(context.Context, string) (remoteengine.Status, error) {
	return f.statusSeen, f.statusErr
}
func (f *fakeEngineDirectory) ControlEngine(_ context.Context, alias string, action remoteengine.EngineAction) (remoteengine.ActionResult, error) {
	if action != remoteengine.ActionStop && action != remoteengine.ActionRestart && action != remoteengine.ActionUpdate {
		return remoteengine.ActionResult{}, remoteengine.ErrUnknownAction
	}
	f.controlled = append(f.controlled, alias+" "+string(action))
	return remoteengine.ActionResult{Succeeded: true, Output: "stopped"}, nil
}
func (f *fakeEngineDirectory) UnlockVault(_ context.Context, alias, passphrase string) error {
	if passphrase != "correct horse battery" {
		return remoteengine.ErrWrongPassphrase
	}
	f.unlocked = append(f.unlocked, alias)
	return nil
}
func (f *fakeEngineDirectory) ForwardTerminalAPI(w http.ResponseWriter, _ *http.Request, alias, remotePath string) error {
	f.forwarded = append(f.forwarded, alias+" "+remotePath)
	w.WriteHeader(http.StatusOK)
	return nil
}
func (f *fakeEngineDirectory) ForwardTerminalStream(w http.ResponseWriter, _ *http.Request, alias string) error {
	f.streamed = append(f.streamed, alias)
	w.WriteHeader(http.StatusSwitchingProtocols)
	return nil
}

func remoteEnginesEngine(directory *fakeEngineDirectory) *echo.Echo {
	engine := echo.New()
	registerRemoteEngineRoutes(engine, RemoteEngineHandlers{Engines: directory, ExpectedOrigin: "http://127.0.0.1:54447"})
	return engine
}

func TestAnAddedEngineIsListedAndAnIncompatibleEngineNamesTheRemoteVersion(t *testing.T) {
	directory := &fakeEngineDirectory{}
	engine := remoteEnginesEngine(directory)
	recorder := send(t, engine, http.MethodPost, "/api/v1/engines", `{"alias":"r540"}`, map[string]string{"Content-Type": "application/json"})
	if recorder.Code != http.StatusCreated {
		t.Fatalf("add = %d %s", recorder.Code, recorder.Body.String())
	}
	recorder = send(t, engine, http.MethodGet, "/api/v1/engines", "", nil)
	if list := decodeAs[api.RemoteEngineList](t, recorder.Body.String()); len(list.Engines) != 1 || list.Engines[0].Alias != "r540" {
		t.Fatalf("list = %s", recorder.Body.String())
	}

	directory.statusErr = &remoteengine.IncompatibleEngineError{Local: "v0.53.0", Remote: "v0.51.0"}
	recorder = send(t, engine, http.MethodGet, "/api/v1/engines/r540/status", "", nil)
	problem := decodeAs[api.Problem](t, recorder.Body.String())
	if recorder.Code != http.StatusConflict || problem.Code != "remote_engine_incompatible" || problem.Detail == nil || *problem.Detail != "v0.51.0" {
		t.Fatalf("incompatible = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestATerminalOperationIsForwardedToTheSamePathOnTheRemoteEngine(t *testing.T) {
	directory := &fakeEngineDirectory{}
	engine := remoteEnginesEngine(directory)
	for _, tc := range []struct{ method, path, remote string }{
		{http.MethodGet, "/api/v1/engines/r540/terminal/sessions", "r540 /api/v1/terminal/sessions"},
		{http.MethodPut, "/api/v1/engines/r540/terminal/sessions/abc/workspace", "r540 /api/v1/terminal/sessions/abc/workspace"},
		{http.MethodPost, "/api/v1/engines/r540/terminal/sessions/abc/reconnect/stop", "r540 /api/v1/terminal/sessions/abc/reconnect/stop"},
		{http.MethodPatch, "/api/v1/engines/r540/terminal/workspaces/w1", "r540 /api/v1/terminal/workspaces/w1"},
	} {
		directory.forwarded = nil
		recorder := send(t, engine, tc.method, tc.path, "", nil)
		if recorder.Code != http.StatusOK || len(directory.forwarded) != 1 || directory.forwarded[0] != tc.remote {
			t.Errorf("%s %s = %d, forwarded %v", tc.method, tc.path, recorder.Code, directory.forwarded)
		}
	}
	if recorder := send(t, engine, http.MethodGet, "/api/v1/engines/-oProxy/terminal/sessions", "", nil); recorder.Code != http.StatusBadRequest {
		t.Fatalf("unsafe alias = %d", recorder.Code)
	}
}

func TestTheForwardedStreamRequiresThisMachinesOrigin(t *testing.T) {
	directory := &fakeEngineDirectory{}
	engine := remoteEnginesEngine(directory)
	foreign := httptest.NewRequest(http.MethodGet, "/engines/r540/terminal/stream?ticket=t", nil)
	foreign.Header.Set("Origin", "http://attacker.example")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, foreign)
	if recorder.Code != http.StatusForbidden || len(directory.streamed) != 0 {
		t.Fatalf("foreign origin = %d, streamed %v", recorder.Code, directory.streamed)
	}
	local := httptest.NewRequest(http.MethodGet, "/engines/r540/terminal/stream?ticket=t", nil)
	local.Header.Set("Origin", "http://127.0.0.1:54447")
	recorder = httptest.NewRecorder()
	engine.ServeHTTP(recorder, local)
	if len(directory.streamed) != 1 || !strings.Contains(directory.streamed[0], "r540") {
		t.Fatalf("local origin streamed %v (status %d)", directory.streamed, recorder.Code)
	}
}

func TestEachReasonAnEngineCannotBeUsedHasItsOwnCode(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{remoteengine.ErrNotAdded, http.StatusNotFound, "remote_engine_not_added"},
		{remoteengine.ErrNotRunning, http.StatusBadGateway, "remote_engine_not_running"},
		{remoteengine.ErrUnproven, http.StatusBadGateway, "remote_engine_unproven"},
	} {
		directory := &fakeEngineDirectory{addErr: tc.err}
		recorder := send(t, remoteEnginesEngine(directory), http.MethodPost, "/api/v1/engines", `{"alias":"r540"}`, map[string]string{"Content-Type": "application/json"})
		if recorder.Code != tc.status || decodeAs[api.Problem](t, recorder.Body.String()).Code != tc.code {
			t.Errorf("%v = %d %s, want %d %s", tc.err, recorder.Code, recorder.Body.String(), tc.status, tc.code)
		}
	}
}

func TestControlRunsTheChosenActionAndRefusesAnUnknownOne(t *testing.T) {
	directory := &fakeEngineDirectory{}
	engine := remoteEnginesEngine(directory)
	headers := map[string]string{"Content-Type": "application/json"}
	recorder := send(t, engine, http.MethodPost, "/api/v1/engines/r540/control", `{"action":"restart"}`, headers)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"succeeded":true`) {
		t.Fatalf("restart = %d %s", recorder.Code, recorder.Body.String())
	}
	if strings.Join(directory.controlled, ",") != "r540 restart" {
		t.Fatalf("controlled = %v", directory.controlled)
	}
	recorder = send(t, engine, http.MethodPost, "/api/v1/engines/r540/control", `{"action":"reboot"}`, headers)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unknown action = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestTheRemoteVaultIsUnlockedAndAWrongPasswordIsNotMistakenForThisMachinesSession(t *testing.T) {
	directory := &fakeEngineDirectory{}
	engine := remoteEnginesEngine(directory)
	headers := map[string]string{"Content-Type": "application/json"}
	recorder := send(t, engine, http.MethodPost, "/api/v1/engines/r540/vault/unlock", `{"passphrase":"correct horse battery"}`, headers)
	if recorder.Code != http.StatusNoContent || strings.Join(directory.unlocked, ",") != "r540" {
		t.Fatalf("unlock = %d %s, unlocked %v", recorder.Code, recorder.Body.String(), directory.unlocked)
	}
	recorder = send(t, engine, http.MethodPost, "/api/v1/engines/r540/vault/unlock", `{"passphrase":"wrong"}`, headers)
	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), `"remote_engine_wrong_passphrase"`) {
		t.Fatalf("wrong password = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestTheForwardedStreamIsReachedOnlyWithThisMachinesSessionAndAnUnlockedVault(t *testing.T) {
	sessions, _, err := session.NewManager(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	credentials, _, err := sessions.JoinOrIssue("", "")
	if err != nil {
		t.Fatal(err)
	}
	var unlocked atomic.Bool
	directory := &fakeEngineDirectory{engines: []string{"r540"}}
	engine := echo.New()
	engine.Use((Security{
		ExpectedHost: "127.0.0.1:54447", ExpectedOrigin: "http://127.0.0.1:54447",
		Sessions: sessions, Unlocked: unlocked.Load,
	}).Middleware)
	registerRemoteEngineRoutes(engine, RemoteEngineHandlers{Engines: directory, ExpectedOrigin: "http://127.0.0.1:54447"})
	stream := func(cookie string) int {
		request := httptest.NewRequest(http.MethodGet, "/engines/r540/terminal/stream?ticket=x", nil)
		request.Host = "127.0.0.1:54447"
		request.Header.Set("Origin", "http://127.0.0.1:54447")
		if cookie != "" {
			request.AddCookie(&http.Cookie{Name: SessionCookie, Value: cookie})
		}
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)
		return recorder.Code
	}

	for _, tc := range []struct {
		name   string
		cookie string
		open   bool
		want   int
	}{
		{"no session", "", true, http.StatusUnauthorized},
		{"an unknown session", "not-a-session", true, http.StatusUnauthorized},
		{"a locked vault", credentials.SessionID, false, http.StatusConflict},
	} {
		unlocked.Store(tc.open)
		if code := stream(tc.cookie); code != tc.want || len(directory.streamed) != 0 {
			t.Fatalf("%s = %d, streamed %v, want %d and nothing streamed", tc.name, code, directory.streamed, tc.want)
		}
	}
	unlocked.Store(true)
	if code := stream(credentials.SessionID); len(directory.streamed) != 1 {
		t.Fatalf("this machine's session = %d, streamed %v", code, directory.streamed)
	}
}
