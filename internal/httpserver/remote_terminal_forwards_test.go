package httpserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/session"
	"sshc/internal/storage"
	"sshc/internal/terminal"
	"sshc/internal/terminalrestore"
)

// echoForwardProcess は、実行先のセッションの役である。転送を求められると、実行先の 127.0.0.1 の
// 空いたポートで、受け取った行をそのまま返す待ち受けを開く（宛先へつなぐ代わり）。
type echoForwardProcess struct {
	*scriptedPTY
	mutex     sync.Mutex
	listeners map[string]net.Listener
	forwards  []terminal.Forward
	asked     []string
}

func (p *echoForwardProcess) Forwards() []terminal.Forward {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return append([]terminal.Forward(nil), p.forwards...)
}

func (p *echoForwardProcess) StartForward(kind, listenPort, destination string) (terminal.Forward, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.asked = append(p.asked, kind+" "+listenPort+" "+destination)
	listener, err := net.Listen("tcp", "127.0.0.1:"+listenPort)
	if err != nil {
		return terminal.Forward{}, err
	}
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer connection.Close()
				line, _ := bufio.NewReader(connection).ReadString('\n')
				_, _ = io.WriteString(connection, "echo:"+line)
			}()
		}
	}()
	forward := terminal.Forward{ID: "pf-1", Kind: kind, Listen: listener.Addr().String(), To: destination, Temporary: true}
	p.listeners[forward.ID] = listener
	p.forwards = append(p.forwards, forward)
	return forward, nil
}

func (p *echoForwardProcess) StopForward(id string) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if listener := p.listeners[id]; listener != nil {
		_ = listener.Close()
		delete(p.listeners, id)
	}
	for index, forward := range p.forwards {
		if forward.ID == id {
			p.forwards = append(p.forwards[:index], p.forwards[index+1:]...)
		}
	}
	return nil
}

// relayWithLoopback は、実行先の役のエンジンへ要求を送り、実行先の 127.0.0.1 へのつなぎを、同じ
// マシンの 127.0.0.1 へのつなぎで代わりにする。
type relayWithLoopback struct {
	relayToEngine
}

func (r *relayWithLoopback) DialRunTargetLoopback(ctx context.Context, _ string, port int) (net.Conn, error) {
	var dialer net.Dialer
	return dialer.DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
}

func runTargetForwardFixture(t *testing.T) (*echo.Echo, session.Credentials, string, *echoForwardProcess) {
	t.Helper()
	manager, bootstrap, err := session.NewManager(bytes.NewReader(bytes.Repeat([]byte{0x6b}, 8192)))
	if err != nil {
		t.Fatal(err)
	}
	remoteCredentials, _, err := manager.BootstrapForSession(bootstrap, "")
	if err != nil {
		t.Fatal(err)
	}
	process := &echoForwardProcess{scriptedPTY: newScriptedPTY(), listeners: map[string]net.Listener{}}
	registry := &terminal.Registry{Limits: func() terminal.Limits { return terminal.Limits{MaxSessions: 4, Scrollback: 1 << 12} }}
	opened, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "web", Title: "web",
		Open: func(context.Context, terminal.Size) (terminal.Process, error) { return process, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		registry.BeginShutdown()
		_ = registry.Wait()
		for id := range process.listeners {
			_ = process.StopForward(id)
		}
	})
	remote := echo.New()
	remote.Use((Security{ExpectedHost: keyTestHost, ExpectedOrigin: "http://" + keyTestHost, Sessions: manager, Unlocked: alwaysUnlocked}).Middleware)
	storageRoot, err := storage.NewWorkspace(storage.OSFileSystem{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	previous, err := terminalrestore.NewService(storageRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := previous.SaveOnStop([]terminalrestore.Session{{Kind: "ssh", Alias: "web", Workspace: "default", PinnedTitle: "build"}}); err != nil {
		t.Fatal(err)
	}
	restore, err := terminalrestore.NewService(storageRoot)
	if err != nil {
		t.Fatal(err)
	}
	registerTerminalRoutes(remote, TerminalHandlers{Registry: registry, RestoreOffers: restore, Tickets: &terminal.Tickets{}, ExpectedOrigin: "http://" + keyTestHost})
	directory := &relayWithLoopback{relayToEngine: relayToEngine{t: t, remote: remote, credentials: remoteCredentials}}
	directory.engines = []string{"r540"}
	local, credentials, _ := localEngineForRunTarget(t, directory)
	waitConnected(t, registry, opened.ID())
	return local, credentials, opened.ID(), process
}

func waitConnected(t *testing.T, registry *terminal.Registry, id string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, view := range registry.Sessions() {
			if view.ID == id && view.State == terminal.StateConnected {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the run target's session did not connect")
}

// startRunTargetLocalForward は、実行先のセッションに local の転送を開き、このマシンの待ち受けのアドレスを返す。
func startRunTargetLocalForward(t *testing.T, local *echo.Echo, credentials session.Credentials, sessionID string) string {
	t.Helper()
	free, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := free.Addr().(*net.TCPAddr).Port
	_ = free.Close()
	request := api.StartTerminalForwardRequest{Kind: "local", ListenPort: port, Destination: stringPointer("10.0.0.5:5432")}
	started := sendKeyRequest(t, local, credentials, http.MethodPost, "/api/v1/engines/r540/terminal/sessions/"+sessionID+"/forwards", mustMarshal(t, request), "")
	if started.Code != http.StatusCreated {
		t.Fatalf("start = %d: %s", started.Code, started.Body.String())
	}
	var listed api.TerminalSessionList
	if err := json.Unmarshal(started.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	want := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	if forwards := *listed.Sessions[0].Forwards; len(forwards) != 1 || forwards[0].Listen != want {
		t.Fatalf("forwards = %+v, want the listener on this machine %s", forwards, want)
	}
	return want
}

func TestARunTargetForwardListensHereAndReachesThePortTheRunTargetOpened(t *testing.T) {
	local, credentials, sessionID, process := runTargetForwardFixture(t)
	want := startRunTargetLocalForward(t, local, credentials, sessionID)
	if got := process.asked; len(got) != 1 || got[0] != "local 0 10.0.0.5:5432" {
		t.Fatalf("the run target was asked %q, want a forward on any free port to the same destination", got)
	}

	connection, err := net.DialTimeout("tcp", want, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = connection.SetDeadline(time.Now().Add(5 * time.Second))
	_, _ = io.WriteString(connection, "hello\n")
	reply, err := bufio.NewReader(connection).ReadString('\n')
	_ = connection.Close()
	if err != nil || reply != "echo:hello\n" {
		t.Fatalf("relayed reply = %q, %v", reply, err)
	}

	listedAgain := sendKeyRequest(t, local, credentials, http.MethodGet, "/api/v1/engines/r540/terminal/sessions", nil, "")
	if !strings.Contains(listedAgain.Body.String(), want) {
		t.Fatalf("the session list shows the run target's port instead of this machine's: %d %s", listedAgain.Code, listedAgain.Body.String())
	}

	stopped := sendKeyRequest(t, local, credentials, http.MethodDelete, "/api/v1/engines/r540/terminal/sessions/"+sessionID+"/forwards/pf-1", nil, "")
	if stopped.Code != http.StatusOK {
		t.Fatalf("stop = %d: %s", stopped.Code, stopped.Body.String())
	}
	if _, err := net.DialTimeout("tcp", want, time.Second); err == nil {
		t.Fatal("this machine still listens after the forward stopped")
	}
}

func TestARunTargetRemoteForwardIsRefusedForNow(t *testing.T) {
	local, credentials, sessionID, _ := runTargetForwardFixture(t)
	request := api.StartTerminalForwardRequest{Kind: "remote", ListenPort: 8080, Destination: stringPointer("127.0.0.1:80")}
	refused := sendKeyRequest(t, local, credentials, http.MethodPost, "/api/v1/engines/r540/terminal/sessions/"+sessionID+"/forwards", mustMarshal(t, request), "")
	if refused.Code != http.StatusConflict || problemCode(t, refused.Body.Bytes()) != "run_target_remote_forward_unavailable" {
		t.Fatalf("remote = %d: %s", refused.Code, refused.Body.String())
	}
}

func TestRemovingARunTargetClosesItsForwardListeners(t *testing.T) {
	local, credentials, sessionID, _ := runTargetForwardFixture(t)
	listen := startRunTargetLocalForward(t, local, credentials, sessionID)
	removed := sendKeyRequest(t, local, credentials, http.MethodDelete, "/api/v1/engines/r540", nil, "")
	if removed.Code != http.StatusOK {
		t.Fatalf("remove = %d: %s", removed.Code, removed.Body.String())
	}
	if _, err := net.DialTimeout("tcp", listen, time.Second); err == nil {
		t.Fatal("this machine still listens for a run target that was removed")
	}
}

func TestTheRestoreOfferOfARunTargetWithAForwardPassesThroughAndKeepsTheListener(t *testing.T) {
	local, credentials, sessionID, _ := runTargetForwardFixture(t)
	listen := startRunTargetLocalForward(t, local, credentials, sessionID)
	offer := sendKeyRequest(t, local, credentials, http.MethodGet, "/api/v1/engines/r540/terminal/restore", nil, "")
	if offer.Code != http.StatusOK || !strings.Contains(offer.Body.String(), `"pinnedTitle":"build"`) {
		t.Fatalf("restore offer = %d: %s, want the offer as the run target wrote it", offer.Code, offer.Body.String())
	}
	connection, err := net.DialTimeout("tcp", listen, time.Second)
	if err != nil {
		t.Fatalf("reading the restore offer closed the forward's listener: %v", err)
	}
	_ = connection.Close()
}
