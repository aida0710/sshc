package remoteengine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"

	"sshc/internal/handoff"
)

const fakeSecret = "the-remote-handoff-secret"

// fakeRemoteEngine は、接続先のエンジンの /cli/ と /api/v1/terminal/ の一部を、本物と
// 同じ規則（秘密、cookie、CSRF、Origin、Sec-Fetch-Site）で答える。
type fakeRemoteEngine struct {
	server   *httptest.Server
	sessions atomic.Int32
	// expired が立つと、今の cookie を一度だけ断る（CLI のセッションの期限切れ）。
	expired atomic.Bool
	mutex   sync.Mutex
	cookie  string
	csrf    string
	seen    []string
}

func newFakeRemoteEngine(t *testing.T) *fakeRemoteEngine {
	t.Helper()
	engine := &fakeRemoteEngine{}
	engine.server = httptest.NewServer(http.HandlerFunc(engine.serve))
	t.Cleanup(engine.server.Close)
	return engine
}

func (f *fakeRemoteEngine) origin() string { return f.server.URL }

func (f *fakeRemoteEngine) handoffDocument(version string) string {
	body, _ := json.Marshal(handoff.Handoff{
		SchemaVersion: handoff.SchemaVersion, URL: f.server.URL, Secret: fakeSecret,
		Owner: handoff.OwnerEngine, PID: 4242, Version: version, ProtocolVersion: handoff.ProtocolVersion,
	})
	return string(body)
}

func (f *fakeRemoteEngine) serve(w http.ResponseWriter, r *http.Request) {
	f.mutex.Lock()
	f.seen = append(f.seen, r.Method+" "+r.URL.Path)
	f.mutex.Unlock()
	switch r.URL.Path {
	case challengePath:
		w.Header().Set(handoff.ProofHeader, handoff.Prove(fakeSecret, r.Header.Get(handoff.ChallengeHeader)))
		w.WriteHeader(http.StatusNoContent)
	case cliSessionPath:
		if r.Header.Get(handoff.HeaderName) != fakeSecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		number := f.sessions.Add(1)
		f.mutex.Lock()
		f.cookie, f.csrf = fmt.Sprintf("cookie-%d", number), fmt.Sprintf("csrf-%d", number)
		cookie, csrf := f.cookie, f.csrf
		f.mutex.Unlock()
		http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: cookie})
		_ = json.NewEncoder(w).Encode(map[string]string{"csrfToken": csrf})
	case cliStatusPath:
		if r.Header.Get(handoff.HeaderName) != fakeSecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(Status{Version: "v9", Vault: true, Unlocked: true, Sessions: 3})
	case "/api/v1/terminal/sessions":
		f.mutex.Lock()
		cookie, csrf := f.cookie, f.csrf
		f.mutex.Unlock()
		sent, _ := r.Cookie(sessionCookieName)
		switch {
		case r.Header.Get("Origin") != f.origin() || r.Header.Get("Sec-Fetch-Site") != "same-origin":
			http.Error(w, "cross_site_request", http.StatusForbidden)
		case sent == nil || sent.Value != cookie || r.Header.Get(csrfHeaderName) != csrf:
			http.Error(w, "invalid_session", http.StatusUnauthorized)
		case f.expired.CompareAndSwap(true, false):
			http.Error(w, "invalid_session", http.StatusUnauthorized)
		default:
			body, _ := io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Set-Cookie", "leaked=1")
			fmt.Fprintf(w, `{"method":%q,"body":%q,"cookies":%q}`, r.Method, string(body), r.Header.Get("Cookie"))
		}
	case RemoteStreamPath:
		if r.Header.Get("Origin") != f.origin() || r.Header.Get("Cookie") != "" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		connection, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer connection.CloseNow()
		kind, message, err := connection.Read(r.Context())
		if err == nil {
			_ = connection.Write(r.Context(), kind, append([]byte("remote:"), message...))
		}
	default:
		http.NotFound(w, r)
	}
}

// handoffLink は fakeLink に、handoffCommand への答えを足したものである。
type handoffLink struct {
	*fakeLink
	handoff  string
	exitCode int
}

func (l *handoffLink) Run(ctx context.Context, command string, limit time.Duration) (RunResult, error) {
	if command == handoffCommand {
		return RunResult{Stdout: []byte(l.handoff), ExitCode: l.exitCode}, nil
	}
	return l.fakeLink.Run(ctx, command, limit)
}

func newEngineManager(t *testing.T, remote *fakeRemoteEngine, remoteVersion string) *Manager {
	t.Helper()
	manager := NewManager(ManagerOptions{
		Store:        newStore(t),
		LocalVersion: "v9",
		Connect: func(context.Context, string) (Link, error) {
			return &handoffLink{fakeLink: newFakeLink(remote.server.URL+"/#x", 0), handoff: remote.handoffDocument(remoteVersion)}, nil
		},
	})
	t.Cleanup(manager.Close)
	return manager
}

func forward(t *testing.T, manager *Manager, method, body string) (int, string, http.Header) {
	t.Helper()
	request := httptest.NewRequest(method, "/api/v1/engines/r540/terminal/sessions", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Cookie", "sshc_session=this-machine")
	request.Header.Set(csrfHeaderName, "this-machine-csrf")
	recorder := httptest.NewRecorder()
	if err := manager.ForwardTerminalAPI(recorder, request, "r540", "/api/v1/terminal/sessions"); err != nil {
		t.Fatal(err)
	}
	return recorder.Code, recorder.Body.String(), recorder.Header()
}

func TestAnAddedEngineIsReachedWithItsOwnCredentialsAndNotThisMachines(t *testing.T) {
	remote := newFakeRemoteEngine(t)
	manager := newEngineManager(t, remote, "v9")
	if err := manager.AddEngine(context.Background(), "r540"); err != nil {
		t.Fatal(err)
	}
	status, body, header := forward(t, manager, http.MethodPost, `{"kind":"shell"}`)
	if status != http.StatusOK || !strings.Contains(body, `"method":"POST"`) || !strings.Contains(body, `"body":"{\"kind\":\"shell\"}"`) {
		t.Fatalf("forwarded = %d %s", status, body)
	}
	if !strings.Contains(body, `"cookies":"sshc_session=cookie-1"`) {
		t.Fatalf("the remote engine received %s, want only its own session cookie", body)
	}
	if header.Get("Set-Cookie") != "" {
		t.Fatalf("a remote cookie reached the browser: %q", header.Get("Set-Cookie"))
	}
	engines, err := manager.Engines()
	if err != nil || len(engines) != 1 || engines[0] != "r540" {
		t.Fatalf("engines = %v, %v", engines, err)
	}
}

func TestAnExpiredRemoteSessionIsIssuedAgainOnceAndTheRequestSentAgain(t *testing.T) {
	remote := newFakeRemoteEngine(t)
	manager := newEngineManager(t, remote, "v9")
	if err := manager.AddEngine(context.Background(), "r540"); err != nil {
		t.Fatal(err)
	}
	remote.expired.Store(true)
	status, body, _ := forward(t, manager, http.MethodGet, "")
	if status != http.StatusOK || !strings.Contains(body, "cookie-2") {
		t.Fatalf("after expiry = %d %s", status, body)
	}
	if got := remote.sessions.Load(); got != 2 {
		t.Fatalf("sessions issued = %d, want 2", got)
	}
}

func TestADifferentVersionOrAStoppedEngineIsNotAdded(t *testing.T) {
	remote := newFakeRemoteEngine(t)
	older := newEngineManager(t, remote, "v8")
	var mismatch *VersionMismatchError
	if err := older.AddEngine(context.Background(), "r540"); !errors.As(err, &mismatch) || mismatch.Remote != "v8" || mismatch.Local != "v9" {
		t.Fatalf("AddEngine with v8 = %v", err)
	}
	if engines, _ := older.Engines(); len(engines) != 0 {
		t.Fatalf("a refused engine was kept: %v", engines)
	}

	stopped := NewManager(ManagerOptions{Store: newStore(t), LocalVersion: "v9", Connect: func(context.Context, string) (Link, error) {
		return &handoffLink{fakeLink: newFakeLink("", 0), exitCode: 1}, nil
	}})
	defer stopped.Close()
	if err := stopped.AddEngine(context.Background(), "r540"); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("AddEngine with no engine running = %v, want ErrNotRunning", err)
	}
}

func TestAnEngineThatCannotProveItsSecretIsRefused(t *testing.T) {
	remote := newFakeRemoteEngine(t)
	manager := NewManager(ManagerOptions{Store: newStore(t), LocalVersion: "v9", Connect: func(context.Context, string) (Link, error) {
		document := strings.Replace(remote.handoffDocument("v9"), fakeSecret, "a-different-secret", 1)
		return &handoffLink{fakeLink: newFakeLink("", 0), handoff: document}, nil
	}})
	defer manager.Close()
	if err := manager.AddEngine(context.Background(), "r540"); !errors.Is(err, ErrUnproven) {
		t.Fatalf("AddEngine = %v, want ErrUnproven", err)
	}
}

func TestOnlyAnAddedEngineIsForwardedToAndRemovingItKeepsTheRemoteRunning(t *testing.T) {
	remote := newFakeRemoteEngine(t)
	manager := newEngineManager(t, remote, "v9")
	request := httptest.NewRequest(http.MethodGet, "/api/v1/engines/r540/terminal/sessions", nil)
	if err := manager.ForwardTerminalAPI(httptest.NewRecorder(), request, "r540", "/api/v1/terminal/sessions"); !errors.Is(err, ErrNotAdded) {
		t.Fatalf("forward before adding = %v, want ErrNotAdded", err)
	}
	if err := manager.AddEngine(context.Background(), "r540"); err != nil {
		t.Fatal(err)
	}
	if err := manager.RemoveEngine("r540"); err != nil {
		t.Fatal(err)
	}
	if err := manager.ForwardTerminalAPI(httptest.NewRecorder(), request, "r540", "/api/v1/terminal/sessions"); !errors.Is(err, ErrNotAdded) {
		t.Fatalf("forward after removing = %v, want ErrNotAdded", err)
	}
}

func TestTheRemoteStatusIsAskedWithTheHandoffSecret(t *testing.T) {
	remote := newFakeRemoteEngine(t)
	manager := newEngineManager(t, remote, "v9")
	if err := manager.AddEngine(context.Background(), "r540"); err != nil {
		t.Fatal(err)
	}
	status, err := manager.EngineStatus(context.Background(), "r540")
	if err != nil || status.Sessions != 3 || !status.Unlocked {
		t.Fatalf("status = %+v, %v", status, err)
	}
}

func TestATerminalStreamReachesTheRemoteEngineWithItsOriginAndNoCookie(t *testing.T) {
	remote := newFakeRemoteEngine(t)
	manager := newEngineManager(t, remote, "v9")
	if err := manager.AddEngine(context.Background(), "r540"); err != nil {
		t.Fatal(err)
	}
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := manager.ForwardTerminalStream(w, r, "r540"); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
		}
	}))
	defer local.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	header := http.Header{}
	header.Set("Origin", local.URL)
	header.Set("Cookie", "sshc_session=this-machine")
	connection, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(local.URL, "http")+"/engines/r540/terminal/stream?ticket=t", &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.CloseNow()
	if err := connection.Write(ctx, websocket.MessageBinary, []byte("ls")); err != nil {
		t.Fatal(err)
	}
	if _, message, err := connection.Read(ctx); err != nil || string(message) != "remote:ls" {
		t.Fatalf("stream answer = %q, %v", message, err)
	}
}
