package remoteengine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"sshc/internal/handoff"
)

// portSquatter は、接続先のエンジンが手放したポートを取った、接続先の別の利用者の
// プロセスである。handoff の秘密を知らない。届いた要求を覚え、何にでも vault_locked で答える。
type portSquatter struct {
	server   *httptest.Server
	mutex    sync.Mutex
	received []string
}

func squatPort(t *testing.T, address string) *portSquatter {
	t.Helper()
	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	squatter := &portSquatter{}
	squatter.server = &httptest.Server{Listener: listener, Config: &http.Server{Handler: http.HandlerFunc(squatter.serve)}}
	squatter.server.Start()
	t.Cleanup(squatter.server.Close)
	return squatter
}

func (s *portSquatter) serve(w http.ResponseWriter, r *http.Request) {
	cookie, _ := r.Cookie(handoff.SessionCookie)
	var body struct {
		Passphrase string `json:"passphrase"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	s.mutex.Lock()
	s.received = append(s.received, fmt.Sprintf("%s %s cookie=%v csrf=%q passphrase=%q",
		r.Method, r.URL.Path, cookie != nil, r.Header.Get(handoff.CSRFHeader), body.Passphrase))
	s.mutex.Unlock()
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusConflict)
	fmt.Fprint(w, `{"code":"vault_locked"}`)
}

// onlyChallenged は、届いた要求が身元の確認だけだったかを確かめる。
func (s *portSquatter) onlyChallenged(t *testing.T) {
	t.Helper()
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if len(s.received) == 0 {
		t.Fatal("the squatter was never asked to prove itself")
	}
	want := fmt.Sprintf("GET %s cookie=false csrf=\"\" passphrase=\"\"", handoff.ChallengePath)
	for _, line := range s.received {
		if line != want {
			t.Errorf("the squatter received %s, want only the challenge", line)
		}
	}
}

func TestAProcessOnTheOldPortReceivesNoCredentialsOrPasswordAfterTheEngineStops(t *testing.T) {
	remote := newFakeRemoteEngine(t)
	manager := newEngineManager(t, remote, "v9")
	if err := manager.AddEngine(context.Background(), "r540"); err != nil {
		t.Fatal(err)
	}
	address := strings.TrimPrefix(remote.server.URL, "http://")
	remote.server.Close()
	squatter := squatPort(t, address)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/engines/r540/terminal/sessions", nil)
	recorder := httptest.NewRecorder()
	if err := manager.ForwardTerminalAPI(recorder, request, "r540", "/api/v1/terminal/sessions"); !errors.Is(err, ErrUnproven) {
		t.Fatalf("forward to the squatter = %v (%d %s), want ErrUnproven", err, recorder.Code, recorder.Body.String())
	}
	if err := manager.UnlockVault(context.Background(), "r540", "the-users-remote-vault-password"); !errors.Is(err, ErrUnproven) {
		t.Fatalf("unlock through the squatter = %v, want ErrUnproven", err)
	}
	if _, err := manager.EngineStatus(context.Background(), "r540"); !errors.Is(err, ErrUnproven) {
		t.Fatalf("status from the squatter = %v, want ErrUnproven", err)
	}
	squatter.onlyChallenged(t)
}

// movingHandoffLink は、接続先のエンジンが起動し直して書き換えた handoff を答える。
type movingHandoffLink struct {
	*fakeLink
	document atomic.Value
}

func (l *movingHandoffLink) Run(ctx context.Context, request RunRequest) (RunResult, error) {
	if request.Command == handoffCommand {
		return RunResult{Stdout: []byte(l.document.Load().(string))}, nil
	}
	return l.fakeLink.Run(ctx, request)
}

func TestAnEngineThatRestartedOnAnotherPortIsFoundAgainFromItsHandoff(t *testing.T) {
	first := newFakeRemoteEngine(t)
	link := &movingHandoffLink{fakeLink: newFakeLink("", 0)}
	link.document.Store(first.handoffDocument("v9"))
	manager := NewManager(ManagerOptions{Store: newStore(t), LocalVersion: "v9", Connect: func(context.Context, string) (Link, error) { return link, nil }})
	defer manager.Close()
	if err := manager.AddEngine(context.Background(), "r540"); err != nil {
		t.Fatal(err)
	}
	first.server.Close()
	restarted := newFakeRemoteEngine(t)
	link.document.Store(restarted.handoffDocument("v9"))

	status, body, _ := forward(t, manager, http.MethodGet, "")
	if status != http.StatusOK || !strings.Contains(body, "cookie-1") {
		t.Fatalf("forward after the restart = %d %s", status, body)
	}
	if got := restarted.sessions.Load(); got != 1 {
		t.Fatalf("sessions issued by the restarted engine = %d, want 1", got)
	}
}

func TestTheRelayCarriesNothingToAProcessOnTheOldPort(t *testing.T) {
	fixture := newManagerFixture(t, newStore(t))
	base := relayBase(t, fixture.open(t))
	address := strings.TrimPrefix(fixture.engine.URL, "http://")
	fixture.engine.Close()
	squatter := squatPort(t, address)

	request, _ := http.NewRequest(http.MethodGet, base+"/", nil)
	request.Header.Set("Cookie", relayCookiePrefix(fixture.manager.hosts["r540"].relay.localPort)+"sshc_session=remote")
	status, body := send(t, request)
	if status != http.StatusBadGateway || !strings.Contains(body, "remote_engine_unproven") {
		t.Fatalf("relay to the squatter = %d %s, want 502 remote_engine_unproven", status, body)
	}
	squatter.onlyChallenged(t)
}

func TestARelayIsNotOpenedWhenTheHandoffNamesAnotherPort(t *testing.T) {
	engine := strictEngine(t)
	other := strictEngine(t)
	manager := NewManager(ManagerOptions{Store: newStore(t), Connect: func(context.Context, string) (Link, error) {
		return &handoffLink{fakeLink: newFakeLink(engine.URL+"/#bootstrap=x", 0), handoff: handoffDocument(other.URL, fakeSecret, "v9")}, nil
	}})
	defer manager.Close()
	if _, err := manager.Open(context.Background(), "r540"); !errors.Is(err, ErrUnproven) {
		t.Fatalf("Open = %v, want ErrUnproven", err)
	}
}
