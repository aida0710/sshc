package remoteengine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// strictEngine は、接続先のエンジンと同じく、Host と Origin が自分の 127.0.0.1:<ポート>
// と完全に一致する要求だけを受け付ける。
func strictEngine(t *testing.T) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		host := strings.TrimPrefix(server.URL, "http://")
		if request.Host != host {
			http.Error(response, "invalid_host "+request.Host, http.StatusForbidden)
			return
		}
		if origin := request.Header.Get("Origin"); origin != "" && origin != server.URL {
			http.Error(response, "cross_site_request "+origin, http.StatusForbidden)
			return
		}
		if request.URL.Path == "/terminal/stream" {
			connection, err := websocket.Accept(response, request, nil)
			if err != nil {
				return
			}
			defer connection.CloseNow()
			kind, message, err := connection.Read(request.Context())
			if err != nil {
				return
			}
			_ = connection.Write(request.Context(), kind, append([]byte("echo:"), message...))
			return
		}
		fmt.Fprintf(response, "%s %s", request.Method, request.URL.Path)
	}))
	t.Cleanup(server.Close)
	return server
}

// fakeLink は、SSH の代わりにこのマシンの TCP を開き、入口のコマンドに決まった出力を返す。
type fakeLink struct {
	entrance string
	exitCode int
	// refuseDial は、SSH サーバーが TCP の転送を断る設定を真似る。
	refuseDial bool
	done       chan struct{}
	once       sync.Once
}

func newFakeLink(entrance string, exitCode int) *fakeLink {
	return &fakeLink{entrance: entrance, exitCode: exitCode, done: make(chan struct{})}
}

func (l *fakeLink) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if l.refuseDial {
		return nil, errors.New("ssh: rejected: administratively prohibited (open failed)")
	}
	var dialer net.Dialer
	return dialer.DialContext(ctx, network, address)
}

func (l *fakeLink) Run(_ context.Context, command string, _ time.Duration) (RunResult, error) {
	if command != EntranceCommand {
		return RunResult{}, fmt.Errorf("unexpected command %q", command)
	}
	return RunResult{Stdout: []byte(l.entrance + "\n"), ExitCode: l.exitCode}, nil
}

func (l *fakeLink) Wait() error { <-l.done; return nil }

func (l *fakeLink) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

type managerFixture struct {
	manager  *Manager
	engine   *httptest.Server
	connects atomic.Int32
	links    chan *fakeLink
}

func newManagerFixture(t *testing.T, store *PortStore) *managerFixture {
	t.Helper()
	fixture := &managerFixture{engine: strictEngine(t), links: make(chan *fakeLink, 8)}
	fixture.manager = NewManager(ManagerOptions{
		Ports: store,
		Connect: func(context.Context, string) (Link, error) {
			fixture.connects.Add(1)
			link := newFakeLink(fixture.engine.URL+"/#bootstrap=one-time", 0)
			fixture.links <- link
			return link, nil
		},
	})
	t.Cleanup(fixture.manager.Close)
	return fixture
}

func (f *managerFixture) open(t *testing.T) string {
	t.Helper()
	opened, err := f.manager.Open(context.Background(), "r540")
	if err != nil {
		t.Fatal(err)
	}
	return opened
}

func relayBase(t *testing.T, opened string) string {
	t.Helper()
	base, _, found := strings.Cut(opened, "/#")
	if !found {
		t.Fatalf("opened URL %q has no fragment", opened)
	}
	return base
}

func send(t *testing.T, request *http.Request) (int, string) {
	t.Helper()
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	return response.StatusCode, string(body)
}

func TestTheRelayURLCarriesTheRemoteEntranceToALocalPort(t *testing.T) {
	fixture := newManagerFixture(t, newPortStore(t))
	opened := fixture.open(t)
	if !strings.HasPrefix(opened, "http://127.0.0.1:") || !strings.HasSuffix(opened, "/#bootstrap=one-time") {
		t.Fatalf("opened = %q", opened)
	}
	if strings.HasPrefix(opened, fixture.engine.URL) {
		t.Fatalf("opened %q points at the remote port instead of the relay", opened)
	}
}

func TestTheRemoteEngineSeesItsOwnHostAndOrigin(t *testing.T) {
	fixture := newManagerFixture(t, newPortStore(t))
	base := relayBase(t, fixture.open(t))

	request, _ := http.NewRequest(http.MethodPost, base+"/api/v1/session/bootstrap", nil)
	request.Header.Set("Origin", base)
	status, body := send(t, request)
	if status != http.StatusOK || body != "POST /api/v1/session/bootstrap" {
		t.Fatalf("relayed POST = %d %q", status, body)
	}
}

func TestTheRelayRefusesAnotherHostOrOriginBeforeRewritingThem(t *testing.T) {
	fixture := newManagerFixture(t, newPortStore(t))
	base := relayBase(t, fixture.open(t))

	rebound, _ := http.NewRequest(http.MethodGet, base+"/", nil)
	rebound.Host = "attacker.example:" + strings.TrimPrefix(base, "http://127.0.0.1:")
	if status, _ := send(t, rebound); status != http.StatusForbidden {
		t.Fatalf("another Host = %d, want 403", status)
	}

	crossSite, _ := http.NewRequest(http.MethodPost, base+"/api/v1/session/bootstrap", nil)
	crossSite.Header.Set("Origin", "http://attacker.example")
	if status, _ := send(t, crossSite); status != http.StatusForbidden {
		t.Fatalf("another Origin = %d, want 403", status)
	}
}

func TestATerminalStreamPassesThroughTheRelay(t *testing.T) {
	fixture := newManagerFixture(t, newPortStore(t))
	base := relayBase(t, fixture.open(t))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	header := http.Header{}
	header.Set("Origin", base)
	connection, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(base, "http")+"/terminal/stream",
		&websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.CloseNow()
	if err := connection.Write(ctx, websocket.MessageBinary, []byte("ls")); err != nil {
		t.Fatal(err)
	}
	_, message, err := connection.Read(ctx)
	if err != nil || string(message) != "echo:ls" {
		t.Fatalf("stream answer = %q, %v", message, err)
	}
}

func TestOpeningAgainKeepsTheSameLocalPortEvenAfterTheEngineRestarts(t *testing.T) {
	store := newPortStore(t)
	fixture := newManagerFixture(t, store)
	first := relayBase(t, fixture.open(t))
	if again := relayBase(t, fixture.open(t)); again != first {
		t.Fatalf("second relay = %s, want %s", again, first)
	}
	fixture.manager.Close()

	restarted := newManagerFixture(t, store)
	if again := relayBase(t, restarted.open(t)); again != first {
		t.Fatalf("relay after restart = %s, want %s", again, first)
	}
}

func TestAnEndedConnectionIsReopenedOnTheNextRequest(t *testing.T) {
	fixture := newManagerFixture(t, newPortStore(t))
	base := relayBase(t, fixture.open(t))
	(<-fixture.links).Close()

	deadline := time.Now().Add(5 * time.Second)
	for {
		request, _ := http.NewRequest(http.MethodGet, base+"/", nil)
		status, _ := send(t, request)
		if status == http.StatusOK && fixture.connects.Load() == 2 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("status %d after %d connects", status, fixture.connects.Load())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestAHostWithoutSSHCAForwardingRefusalAndAFailedConnectionAreToldApart(t *testing.T) {
	store := newPortStore(t)
	missing := NewManager(ManagerOptions{Ports: store, Connect: func(context.Context, string) (Link, error) {
		return newFakeLink("", notInstalledExit), nil
	}})
	defer missing.Close()
	if _, err := missing.Open(context.Background(), "r540"); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("missing sshc = %v, want ErrNotInstalled", err)
	}

	engine := strictEngine(t)
	prohibited := NewManager(ManagerOptions{Ports: store, Connect: func(context.Context, string) (Link, error) {
		link := newFakeLink(engine.URL+"/#bootstrap=x", 0)
		link.refuseDial = true
		return link, nil
	}})
	defer prohibited.Close()
	if _, err := prohibited.Open(context.Background(), "r540"); !errors.Is(err, ErrForwardingRefused) {
		t.Fatalf("forwarding refused = %v, want ErrForwardingRefused", err)
	}

	refused := errors.New("handshake failed")
	unreachable := NewManager(ManagerOptions{Ports: store, Connect: func(context.Context, string) (Link, error) {
		return nil, refused
	}})
	defer unreachable.Close()
	_, err := unreachable.Open(context.Background(), "r540")
	var connectErr *ConnectError
	if !errors.As(err, &connectErr) || !errors.Is(err, refused) {
		t.Fatalf("failed connection = %v, want a ConnectError around %v", err, refused)
	}
}

func TestAConnectionFromAnotherUserIsClosedUnread(t *testing.T) {
	fixture := newManagerFixture(t, newPortStore(t))
	fixture.manager.options.PeerMayBelongToAnotherUser = func(_, _ netip.AddrPort) bool { return true }
	base := relayBase(t, fixture.open(t))

	request, _ := http.NewRequest(http.MethodGet, base+"/", nil)
	if response, err := http.DefaultClient.Do(request); err == nil {
		response.Body.Close()
		t.Fatalf("a connection from another user got %d", response.StatusCode)
	}
}
