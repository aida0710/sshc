package httpserver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/platform"
	"sshc/internal/terminal"
)

// scriptedPTY は、テストが押し込んだものを返す擬似端末である。実プロセスは
// 一つも起きない。
type scriptedPTY struct {
	mutex   sync.Mutex
	pending [][]byte
	written []byte
	ready   chan struct{}
	done    chan struct{}
	info    terminal.ExitInfo
	closed  bool
}

func TestRemoteWorkingDirectoryAndShellQuote(t *testing.T) {
	t.Parallel()
	directory, err := remoteWorkingDirectory("/srv/project's files")
	if err != nil {
		t.Fatal(err)
	}
	if quoted := quotePOSIXShell(directory); quoted != `'/srv/project'"'"'s files'` {
		t.Fatalf("quoted directory = %q", quoted)
	}
	for _, candidate := range []string{"relative", "/srv/../etc", "/srv\nwhoami"} {
		if _, err := remoteWorkingDirectory(candidate); err == nil {
			t.Fatalf("accepted unsafe directory %q", candidate)
		}
	}
}

func newScriptedPTY() *scriptedPTY {
	return &scriptedPTY{ready: make(chan struct{}, 1), done: make(chan struct{})}
}

func (p *scriptedPTY) feed(chunk string) {
	p.mutex.Lock()
	p.pending = append(p.pending, []byte(chunk))
	p.mutex.Unlock()
	select {
	case p.ready <- struct{}{}:
	default:
	}
}

func (p *scriptedPTY) exit(info terminal.ExitInfo) {
	p.mutex.Lock()
	if p.closed {
		p.mutex.Unlock()
		return
	}
	p.closed, p.info = true, info
	p.mutex.Unlock()
	close(p.done)
}

func (p *scriptedPTY) Read(b []byte) (int, error) {
	for {
		p.mutex.Lock()
		if len(p.pending) > 0 {
			chunk := p.pending[0]
			p.pending = p.pending[1:]
			p.mutex.Unlock()
			return copy(b, chunk), nil
		}
		p.mutex.Unlock()
		select {
		case <-p.ready:
		case <-p.done:
			return 0, io.EOF
		}
	}
}

func (p *scriptedPTY) Write(b []byte) (int, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.written = append(p.written, b...)
	return len(b), nil
}

func (p *scriptedPTY) WriteExact(_ context.Context, b []byte) error {
	_, err := p.Write(b)
	return err
}

func (p *scriptedPTY) keystrokes() string {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return string(p.written)
}

func (p *scriptedPTY) Resize(terminal.Size) error { return nil }
func (p *scriptedPTY) Hangup() error              { p.exit(terminal.ExitInfo{Signal: "hangup"}); return nil }
func (p *scriptedPTY) Wait() terminal.ExitInfo {
	<-p.done
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.info
}
func (p *scriptedPTY) Close() error { return nil }
func (p *scriptedPTY) ForceClose() error {
	p.exit(terminal.ExitInfo{Signal: "killed"})
	return nil
}

type scriptedStarter struct {
	mutex     sync.Mutex
	processes []*scriptedPTY
	commands  []terminal.Command
}

type readinessPTY struct {
	*scriptedPTY
	connected chan error
}

func (p *readinessPTY) Ready() <-chan error { return p.connected }

type forwardingReadinessPTY struct {
	*readinessPTY
	forwards []terminal.Forward
	next     int
}

func (p *forwardingReadinessPTY) Forwards() []terminal.Forward { return p.forwards }
func (p *forwardingReadinessPTY) StartForward(kind, listenPort, destination string) (terminal.Forward, error) {
	p.next++
	forward := terminal.Forward{
		ID: fmt.Sprintf("pf-%d", p.next), Kind: kind, Listen: "127.0.0.1:" + listenPort,
		To: destination, Temporary: true,
	}
	p.forwards = append(p.forwards, forward)
	return forward, nil
}
func (p *forwardingReadinessPTY) StopForward(id string) error {
	for index, forward := range p.forwards {
		if forward.ID == id {
			p.forwards = append(p.forwards[:index], p.forwards[index+1:]...)
			return nil
		}
	}
	return terminal.ErrForwardNotFound
}

func (s *scriptedStarter) Start(_ context.Context, command terminal.Command, _ terminal.Size) (terminal.Process, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	process := newScriptedPTY()
	s.processes = append(s.processes, process)
	s.commands = append(s.commands, command)
	return process, nil
}

func (s *scriptedStarter) last() *scriptedPTY {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.processes[len(s.processes)-1]
}

func (s *scriptedStarter) opened() []terminal.Command {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return append([]terminal.Command(nil), s.commands...)
}

// terminalFixture は、端末のルートだけを載せた本物のサーバーである。
//
// Security ミドルウェアはここでは要らない。このスイートが見るのは、その手前を
// 通り抜けたあとにハンドラ自身が何を拒否するかだからだ。トランスポートの規則は
// internal/acceptance が全ルートに対してまとめて表明している。
type terminalFixture struct {
	server   *httptest.Server
	starter  *scriptedStarter
	registry *terminal.Registry
	origin   string
	mutex    sync.Mutex
	// connected は、プロセス内 SSH に渡された alias である。
	connected  []string
	ssh        []*scriptedPTY
	sshReady   []*readinessPTY
	asyncSSH   bool
	forwarding bool
	forwarders []*forwardingReadinessPTY
	recorded   []string
	resumed    []string
}

func (f *terminalFixture) connect(alias string) terminal.Process {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.connected = append(f.connected, alias)
	process := newScriptedPTY()
	f.ssh = append(f.ssh, process)
	if f.forwarding {
		ready := &readinessPTY{scriptedPTY: process, connected: make(chan error, 1)}
		forwarder := &forwardingReadinessPTY{readinessPTY: ready}
		f.forwarders = append(f.forwarders, forwarder)
		ready.connected <- nil
		close(ready.connected)
		return forwarder
	}
	if f.asyncSSH {
		ready := &readinessPTY{scriptedPTY: process, connected: make(chan error, 1)}
		f.sshReady = append(f.sshReady, ready)
		return ready
	}
	return process
}

func TestTemporaryForwardRoutesStartListAndStopOneListener(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 4, Scrollback: 1 << 12})
	fixture.forwarding = true
	response, body := fixture.do(t, http.MethodPost, "/api/v1/terminal/sessions", `{"kind":"ssh","alias":"bastion"}`)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("open = %d: %s", response.StatusCode, body)
	}
	var opened api.OpenTerminalSessionResponse
	if err := json.Unmarshal([]byte(body), &opened); err != nil {
		t.Fatal(err)
	}

	path := "/api/v1/terminal/sessions/" + opened.Session.Id + "/forwards"
	response, body = fixture.do(t, http.MethodPost, path, `{"kind":"local","listenPort":18080,"destination":"db.internal:5432"}`)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("start = %d: %s", response.StatusCode, body)
	}
	var listed api.TerminalSessionList
	if err := json.Unmarshal([]byte(body), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Sessions) != 1 || listed.Sessions[0].Forwards == nil || len(*listed.Sessions[0].Forwards) != 1 {
		t.Fatalf("sessions = %#v", listed.Sessions)
	}
	forward := (*listed.Sessions[0].Forwards)[0]
	if forward.Id == "" || forward.Listen != "127.0.0.1:18080" || !forward.Temporary {
		t.Fatalf("forward = %#v", forward)
	}

	response, body = fixture.do(t, http.MethodDelete, path+"/"+forward.Id, "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("stop = %d: %s", response.StatusCode, body)
	}
	listed = api.TerminalSessionList{}
	if err := json.Unmarshal([]byte(body), &listed); err != nil {
		t.Fatal(err)
	}
	if listed.Sessions[0].Forwards != nil && len(*listed.Sessions[0].Forwards) != 0 {
		t.Fatalf("forwards after stop = %#v", listed.Sessions[0].Forwards)
	}
}

func TestTemporaryForwardRouteRejectsInvalidAndLocalShellRequests(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 4, Scrollback: 1 << 12})
	id, _ := fixture.openShell(t)
	path := "/api/v1/terminal/sessions/" + id + "/forwards"
	for _, body := range []string{
		`{"kind":"remote","listenPort":8080,"destination":"db:5432"}`,
		`{"kind":"local","listenPort":0,"destination":"db:5432"}`,
		`{"kind":"local","listenPort":8080}`,
		`{"kind":"dynamic","listenPort":1080,"destination":"db:5432"}`,
	} {
		response, responseBody := fixture.do(t, http.MethodPost, path, body)
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("request %s = %d: %s", body, response.StatusCode, responseBody)
		}
	}
	response, body := fixture.do(t, http.MethodPost, path, `{"kind":"dynamic","listenPort":1080}`)
	if response.StatusCode != http.StatusConflict || !strings.Contains(body, "terminal_forward_unavailable") {
		t.Fatalf("shell forward = %d: %s", response.StatusCode, body)
	}
}

func (f *terminalFixture) record(alias string) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.recorded = append(f.recorded, alias)
}

func newTerminalFixture(t *testing.T, limits terminal.Limits) *terminalFixture {
	t.Helper()
	starter := &scriptedStarter{}
	registry := &terminal.Registry{
		Start:  starter,
		Limits: func() terminal.Limits { return limits },
	}
	fixture := &terminalFixture{starter: starter, registry: registry}

	// ハンドラは期待するオリジンと完全一致を求めるので、待ち受けるアドレスを
	// 先に確定させてからルートを登録する。
	server := httptest.NewUnstartedServer(nil)
	fixture.origin = "http://" + server.Listener.Addr().String()

	engine := echo.New()
	registerTerminalRoutes(engine, TerminalHandlers{
		Registry:       registry,
		Tickets:        &terminal.Tickets{},
		Shell:          func() (string, error) { return "/bin/zsh", nil },
		StartDirectory: func() string { return "/home/tester" },
		// SSH はプロセス内で通信する。この検査は PTY の継ぎ目を見ているので、
		// 開いたことだけを記録する接続で足りる。
		Connect: func(_ context.Context, alias string, _ terminal.Size) (terminal.Process, error) {
			return fixture.connect(alias), nil
		},
		ConnectAgent: func(_ context.Context, alias string, kind terminal.AgentKind, reference string, _ terminal.Size) (terminal.Process, error) {
			fixture.mutex.Lock()
			fixture.resumed = append(fixture.resumed, string(kind)+":"+reference)
			fixture.mutex.Unlock()
			return fixture.connect(alias), nil
		},
		Connected:      fixture.record,
		ExpectedOrigin: fixture.origin,
	})
	server.Config.Handler = engine
	server.Start()
	fixture.server = server
	t.Cleanup(func() {
		fixture.server.Close()
		registry.BeginShutdown()
		_ = registry.Wait()
	})
	return fixture
}

func (f *terminalFixture) do(t *testing.T, method, path string, body string) (*http.Response, string) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request, err := http.NewRequest(method, f.server.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	contents, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response, string(contents)
}

// openShell はローカルシェルを一本開き、その ID とチケットを返す。
func (f *terminalFixture) openShell(t *testing.T) (string, string) {
	t.Helper()
	response, body := f.do(t, http.MethodPost, "/api/v1/terminal/sessions", `{"kind":"shell"}`)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("open = %d: %s", response.StatusCode, body)
	}
	var opened api.OpenTerminalSessionResponse
	if err := json.Unmarshal([]byte(body), &opened); err != nil {
		t.Fatal(err)
	}
	return opened.Session.Id, opened.StreamTicket
}

func (f *terminalFixture) dial(t *testing.T, ticket string) (*websocket.Conn, *http.Response) {
	t.Helper()
	url := strings.Replace(f.server.URL, "http://", "ws://", 1) + StreamPath + "?ticket=" + ticket
	connection, response, err := websocket.Dial(context.Background(), url, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{f.origin}},
	})
	if err != nil {
		return nil, response
	}
	t.Cleanup(func() { _ = connection.CloseNow() })
	return connection, response
}

func TestOpeningASessionReturnsATicketAndListsIt(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 4, Scrollback: 1 << 12})

	id, ticket := fixture.openShell(t)
	if id == "" || ticket == "" {
		t.Fatalf("id = %q, ticket = %q", id, ticket)
	}

	response, body := fixture.do(t, http.MethodGet, "/api/v1/terminal/sessions", "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("list = %d: %s", response.StatusCode, body)
	}
	var listed api.TerminalSessionList
	if err := json.Unmarshal([]byte(body), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Sessions) != 1 || listed.Sessions[0].Id != id {
		t.Fatalf("sessions = %#v", listed.Sessions)
	}
	if listed.Sessions[0].Kind != api.TerminalSessionKindShell || listed.Sessions[0].Alias != nil {
		t.Fatalf("a local shell is not an ssh connection: %#v", listed.Sessions[0])
	}
	if listed.Sessions[0].State != api.TerminalSessionState("connected") || listed.Sessions[0].Problem != "" {
		t.Fatalf("state/problem = %q/%q", listed.Sessions[0].State, listed.Sessions[0].Problem)
	}
	if listed.MaxSessions != 4 {
		t.Fatalf("maxSessions = %d", listed.MaxSessions)
	}

	// ローカルシェルはログインシェルとして起動する。そう伝える手段は OS ごとに
	// 違うので、表明も分けてある。
	opened := fixture.starter.opened()
	if len(opened) != 1 {
		t.Fatalf("command = %#v", opened)
	}
	assertOpenedAsALoginShell(t, opened[0])

	// 始まる場所は設定が決める。継ぐと、エンジンを起動したものが
	// たまたま居た場所でシェルが始まる。デスクトップのネイティブ層から起こせば
	// その端末の作業ディレクトリ、launchd から起こせば `/`。利用者はそのどれも選んでいない。
	if opened[0].Dir != "/home/tester" {
		t.Fatalf("the shell started in %q, want the home directory", opened[0].Dir)
	}
}

func TestLocalShellProfileUsesStoredDefaultAndOneShotOverride(t *testing.T) {
	handlers := TerminalHandlers{
		ShellProfiles: func() []platform.ShellProfile {
			return []platform.ShellProfile{
				{ID: "default", Label: "sh", Path: "/bin/sh", Argv0: "-sh"},
				{ID: "fish", Label: "fish", Path: "/usr/bin/fish", Argv0: "-fish"},
				{ID: "zsh", Label: "zsh", Path: "/bin/zsh", Argv0: "-zsh"},
			}
		},
		DefaultShellProfile: func() string { return "fish" },
	}
	stored, err := handlers.shellSpec(nil, terminal.Size{Cols: 80, Rows: 24})
	if err != nil || stored.Command.Path != "/usr/bin/fish" || stored.Command.Argv0 != "-fish" {
		t.Fatalf("stored profile = %#v, %v", stored.Command, err)
	}
	oneShot := "zsh"
	overridden, err := handlers.shellSpec(&oneShot, terminal.Size{Cols: 80, Rows: 24})
	if err != nil || overridden.Command.Path != "/bin/zsh" {
		t.Fatalf("one-shot profile = %#v, %v", overridden.Command, err)
	}
	handlers.DefaultShellProfile = func() string { return "powershell" }
	fallback, err := handlers.shellSpec(nil, terminal.Size{Cols: 80, Rows: 24})
	if err != nil || fallback.Command.Path != "/bin/sh" {
		t.Fatalf("cross-platform fallback = %#v, %v", fallback.Command, err)
	}
	unsafe := "/bin/sh -c id"
	if _, err := handlers.shellSpec(&unsafe, terminal.Size{}); !errors.Is(err, platform.ErrUnknownShellProfile) {
		t.Fatalf("unsafe profile = %v", err)
	}
}

func TestStartupSnippetWaitsForEverySSHConnectionToBecomeReady(t *testing.T) {
	var opened []*readinessPTY
	handlers := TerminalHandlers{
		Connect: func(_ context.Context, _ string, _ terminal.Size) (terminal.Process, error) {
			process := &readinessPTY{scriptedPTY: newScriptedPTY(), connected: make(chan error, 1)}
			opened = append(opened, process)
			return process, nil
		},
		Startup: func(alias string) (string, bool) {
			if alias != "production" {
				t.Fatalf("startup alias = %q", alias)
			}
			return "cd /srv/app", true
		},
	}
	alias := "production"
	spec, err := handlers.spec(terminal.KindSSH, &alias, nil, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}

	for attempt := 0; attempt < 2; attempt++ {
		process, err := spec.Open(context.Background(), terminal.Size{Cols: 80, Rows: 24})
		if err != nil {
			t.Fatal(err)
		}
		candidate := opened[attempt]
		if got := candidate.keystrokes(); got != "" {
			t.Fatalf("attempt %d wrote before ready: %q", attempt, got)
		}
		candidate.connected <- nil
		close(candidate.connected)
		deadline := time.Now().Add(time.Second)
		for candidate.keystrokes() == "" && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if got := candidate.keystrokes(); got != "cd /srv/app\r" {
			t.Fatalf("attempt %d startup = %q", attempt, got)
		}
		candidate.exit(terminal.ExitInfo{})
		_ = process.Close()
	}
}

func TestSSHSessionLifetimePreservesForwarderCapability(t *testing.T) {
	underlying := &forwardingReadinessPTY{
		readinessPTY: &readinessPTY{scriptedPTY: newScriptedPTY(), connected: make(chan error, 1)},
		forwards:     []terminal.Forward{{Kind: terminal.ForwardLocal, Listen: "127.0.0.1:9000", To: "db:5432"}},
	}
	handlers := TerminalHandlers{
		Connect: func(context.Context, string, terminal.Size) (terminal.Process, error) {
			return underlying, nil
		},
	}
	alias := "production"
	spec, err := handlers.spec(terminal.KindSSH, &alias, nil, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	process, err := spec.Open(context.Background(), terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	forwarder, ok := process.(terminal.Forwarder)
	if !ok {
		t.Fatal("SSH lifetime wrapper dropped the Forwarder capability")
	}
	if forwards := forwarder.Forwards(); len(forwards) != 1 || forwards[0] != underlying.forwards[0] {
		t.Fatalf("forwards = %#v, want %#v", forwards, underlying.forwards)
	}
	controller, ok := process.(terminal.ForwardController)
	if !ok {
		t.Fatal("SSH lifetime wrapper dropped the ForwardController capability")
	}
	added, err := controller.StartForward(terminal.ForwardDynamic, "1080", "")
	if err != nil || added.ID == "" {
		t.Fatalf("start forward = %#v, %v", added, err)
	}
	if err := controller.StopForward(added.ID); err != nil {
		t.Fatalf("stop forward: %v", err)
	}
	underlying.connected <- nil
	close(underlying.connected)
	underlying.exit(terminal.ExitInfo{})
	_ = process.Close()
}

// 上限に達した状態で開こうとした要求は拒否する。暗黙に古いセッションを
// 閉じることはしない。
func TestOpeningPastTheLimitIsRefusedAndClosesNothing(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 2, Scrollback: 1 << 12})

	first, _ := fixture.openShell(t)
	second, _ := fixture.openShell(t)

	response, body := fixture.do(t, http.MethodPost, "/api/v1/terminal/sessions", `{"kind":"shell"}`)
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("the third open = %d: %s", response.StatusCode, body)
	}
	if !strings.Contains(body, "terminal_session_limit") {
		t.Fatalf("problem = %s", body)
	}
	for _, id := range []string{first, second} {
		if session, ok := fixture.registry.Lookup(id); !ok || !session.Live() {
			t.Fatalf("the refusal closed session %q", id)
		}
	}
}

func TestOpeningAnSSHSessionNeedsASafeAlias(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 4, Scrollback: 1 << 12})

	for name, body := range map[string]string{
		"no alias":     `{"kind":"ssh"}`,
		"unsafe alias": `{"kind":"ssh","alias":"-oProxyCommand=id"}`,
		"unknown kind": `{"kind":"telnet"}`,
	} {
		t.Run(name, func(t *testing.T) {
			response, contents := fixture.do(t, http.MethodPost, "/api/v1/terminal/sessions", body)
			if response.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d: %s", response.StatusCode, contents)
			}
		})
	}
	if opened := fixture.starter.opened(); len(opened) != 0 {
		t.Fatalf("a refused request still opened %#v", opened)
	}
}

func TestASuccessfulSSHSessionIsRecordedAfterItCanBeStreamed(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 4, Scrollback: 1 << 12})
	response, body := fixture.do(t, http.MethodPost, "/api/v1/terminal/sessions", `{"kind":"ssh","alias":"bastion"}`)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("open = %d: %s", response.StatusCode, body)
	}
	fixture.mutex.Lock()
	defer fixture.mutex.Unlock()
	if len(fixture.recorded) != 1 || fixture.recorded[0] != "bastion" {
		t.Fatalf("recorded = %#v", fixture.recorded)
	}
}

func TestAsynchronousSSHIsNotRecordedOrConnectedBeforeReady(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 4, Scrollback: 1 << 12})
	fixture.asyncSSH = true
	response, body := fixture.do(t, http.MethodPost, "/api/v1/terminal/sessions", `{"kind":"ssh","alias":"bastion"}`)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("open = %d: %s", response.StatusCode, body)
	}
	var opened api.OpenTerminalSessionResponse
	if err := json.Unmarshal([]byte(body), &opened); err != nil {
		t.Fatal(err)
	}
	if opened.Session.State != api.TerminalSessionState("connecting") {
		t.Fatalf("initial state = %q, want connecting", opened.Session.State)
	}
	fixture.mutex.Lock()
	if len(fixture.recorded) != 0 {
		t.Fatalf("recorded before Ready = %#v", fixture.recorded)
	}
	ready := fixture.sshReady[0]
	fixture.mutex.Unlock()

	ready.connected <- nil
	close(ready.connected)
	waitUntil(t, func() bool {
		fixture.mutex.Lock()
		defer fixture.mutex.Unlock()
		return len(fixture.recorded) == 1 && fixture.recorded[0] == "bastion"
	})
	waitUntil(t, func() bool {
		session, ok := fixture.registry.Lookup(opened.Session.Id)
		return ok && session.View().State == terminal.StateConnected
	})
}

func TestDescribeSessionIncludesSSHConnectionProgress(t *testing.T) {
	described := describeSession(terminal.View{
		ID: "one", Kind: terminal.KindSSH, Alias: "destination", Title: "destination",
		Started: time.Now(), State: terminal.StateConnecting,
		Progress: &terminal.ConnectionProgress{
			Phase: terminal.ConnectionAuthenticating, Alias: "bastion", HostName: "192.0.2.10",
			User: "ops", Hop: 1, Hops: 2,
		},
	})
	if described.Progress == nil || described.Progress.Phase != api.Authenticating ||
		described.Progress.Alias != "bastion" || described.Progress.Hop != 1 || described.Progress.Hops != 2 {
		t.Fatalf("progress = %+v", described.Progress)
	}
}

func TestExitedSSHSessionCanBeExplicitlyReconnectedInPlace(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 4, Scrollback: 1 << 12})
	response, body := fixture.do(t, http.MethodPost, "/api/v1/terminal/sessions", `{"kind":"ssh","alias":"bastion"}`)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("open = %d: %s", response.StatusCode, body)
	}
	var opened api.OpenTerminalSessionResponse
	if err := json.Unmarshal([]byte(body), &opened); err != nil {
		t.Fatal(err)
	}
	fixture.mutex.Lock()
	first := fixture.ssh[0]
	fixture.mutex.Unlock()
	first.feed("before manual reconnect\r\n")
	waitUntil(t, func() bool {
		session, ok := fixture.registry.Lookup(opened.Session.Id)
		return ok && strings.Contains(string(session.Snapshot()), "before manual reconnect")
	})
	first.exit(terminal.ExitInfo{Code: 255})
	waitUntil(t, func() bool {
		session, ok := fixture.registry.Lookup(opened.Session.Id)
		return ok && session.Exit() != nil
	})

	response, body = fixture.do(t, http.MethodPost, "/api/v1/terminal/sessions/"+opened.Session.Id+"/reconnect", "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("reconnect = %d: %s", response.StatusCode, body)
	}
	var listed api.TerminalSessionList
	if err := json.Unmarshal([]byte(body), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Sessions) != 1 || listed.Sessions[0].Id != opened.Session.Id ||
		listed.Sessions[0].State != api.TerminalSessionState("connected") || listed.Sessions[0].Exited != nil {
		t.Fatalf("sessions after reconnect = %#v", listed.Sessions)
	}
	fixture.mutex.Lock()
	if len(fixture.connected) != 2 || fixture.connected[1] != "bastion" {
		t.Fatalf("connections = %#v", fixture.connected)
	}
	second := fixture.ssh[1]
	fixture.mutex.Unlock()

	ticket := fixture.newTicket(t, opened.Session.Id)
	connection, _ := fixture.dial(t, ticket)
	if connection == nil {
		t.Fatal("the reconnected stream was unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	kind, payload, err := connection.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if kind != websocket.MessageBinary || !strings.Contains(string(payload), "before manual reconnect") ||
		!strings.Contains(string(payload), "新しいシェル") {
		t.Fatalf("replayed = %v %q", kind, payload)
	}
	second.exit(terminal.ExitInfo{})
}

func TestExplicitReconnectRefusesALocalShell(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 4, Scrollback: 1 << 12})
	id, _ := fixture.openShell(t)
	fixture.starter.last().exit(terminal.ExitInfo{})
	waitUntil(t, func() bool {
		session, ok := fixture.registry.Lookup(id)
		return ok && session.Exit() != nil
	})
	response, body := fixture.do(t, http.MethodPost, "/api/v1/terminal/sessions/"+id+"/reconnect", "")
	if response.StatusCode != http.StatusConflict || !strings.Contains(body, "terminal_reconnect_unavailable") {
		t.Fatalf("local reconnect = %d: %s", response.StatusCode, body)
	}
}

func TestClosingForceStopsAndForgetsInOneRequest(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 4, Scrollback: 1 << 12})
	id, _ := fixture.openShell(t)
	session, ok := fixture.registry.Lookup(id)
	if !ok {
		t.Fatal("the opened session disappeared")
	}

	response, body := fixture.do(t, http.MethodDelete, "/api/v1/terminal/sessions/"+id, "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("close = %d: %s", response.StatusCode, body)
	}
	if _, ok := fixture.registry.Lookup(id); ok {
		t.Fatal("the closed session remained in the list")
	}
	// 強制停止後のpump回収まで確認し、一覧だけを先に消してprocessを漏らしていない
	// ことを保証する。
	select {
	case <-session.Done():
	case <-time.After(30 * time.Second):
		t.Fatal("the session pump did not finish after force close")
	}
	if session.Exit() == nil {
		t.Fatal("the finished session has no exit status")
	}

	if response, _ := fixture.do(t, http.MethodDelete, "/api/v1/terminal/sessions/"+id, ""); response.StatusCode != http.StatusNotFound {
		t.Fatalf("closing a missing session = %d, want 404", response.StatusCode)
	}
}

// チケットは使い捨てである。二度目も、別のユーザーが考えた値も、アップグレードせずに
// 403 を返す。101 を返してから閉じると、拒否の理由が「繋がったのに切れた」と
// 区別できなくなる。
func TestTheStreamRefusesAnythingButOneFreshTicket(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 4, Scrollback: 1 << 12})
	_, ticket := fixture.openShell(t)

	connection, _ := fixture.dial(t, ticket)
	if connection == nil {
		t.Fatal("a fresh ticket was refused")
	}
	_ = connection.CloseNow()

	for name, presented := range map[string]string{
		"already spent": ticket,
		"invented":      strings.Repeat("A", 43),
		"empty":         "",
	} {
		t.Run(name, func(t *testing.T) {
			connection, response := fixture.dial(t, presented)
			if connection != nil {
				t.Fatal("the upgrade was accepted")
			}
			if response == nil || response.StatusCode != http.StatusForbidden {
				t.Fatalf("response = %#v, want 403", response)
			}
		})
	}
}

func TestTheStreamRefusesAnotherOrigin(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 4, Scrollback: 1 << 12})
	_, ticket := fixture.openShell(t)

	url := strings.Replace(fixture.server.URL, "http://", "ws://", 1) +
		StreamPath + "?ticket=" + ticket
	connection, response, err := websocket.Dial(context.Background(), url, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"https://evil.example"}},
	})
	if err == nil {
		_ = connection.CloseNow()
		t.Fatal("a cross-origin upgrade was accepted")
	}
	if response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("response = %#v, want 403", response)
	}
}

// バイナリフレームは PTY の生バイト列である。base64 を挟まない。
func TestTheStreamCarriesOutputKeystrokesAndTheExit(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 4, Scrollback: 1 << 12})
	id, ticket := fixture.openShell(t)
	process := fixture.starter.last()

	connection, _ := fixture.dial(t, ticket)
	if connection == nil {
		t.Fatal("the stream refused a fresh ticket")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	process.feed("hello from the pty\r\n")
	kind, payload, err := connection.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if kind != websocket.MessageBinary || string(payload) != "hello from the pty\r\n" {
		t.Fatalf("frame = %v %q", kind, payload)
	}

	// 打鍵はそのまま PTY へ。
	if err := connection.Write(ctx, websocket.MessageBinary, []byte("echo hi\r")); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool { return process.keystrokes() == "echo hi\r" })

	// 終了はテキストフレームで届く。
	process.exit(terminal.ExitInfo{Code: 42})
	for {
		kind, payload, err = connection.Read(ctx)
		if err != nil {
			t.Fatalf("the exit never arrived: %v", err)
		}
		if kind != websocket.MessageText {
			continue
		}
		var message struct {
			Exit struct {
				Code   int    `json:"code"`
				Signal string `json:"signal"`
			} `json:"exit"`
		}
		if err := json.Unmarshal(payload, &message); err != nil {
			t.Fatal(err)
		}
		if message.Exit.Code != 42 {
			t.Fatalf("exit = %#v", message.Exit)
		}
		break
	}

	// 終了しても一覧に残る。ssh が接続できなかった理由が読めるのはそこだけである。
	_, body := fixture.do(t, http.MethodGet, "/api/v1/terminal/sessions", "")
	var listed api.TerminalSessionList
	if err := json.Unmarshal([]byte(body), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Sessions) != 1 || listed.Sessions[0].Id != id || listed.Sessions[0].Exited == nil {
		t.Fatalf("sessions = %#v", listed.Sessions)
	}
	if listed.Sessions[0].Exited.Code != 42 {
		t.Fatalf("exit = %#v", listed.Sessions[0].Exited)
	}
	if listed.Sessions[0].State != api.TerminalSessionState("exited") {
		t.Fatalf("state = %q, want exited", listed.Sessions[0].State)
	}
}

// WebSocket が切れてもセッションは死なない。同じ ID へ新しいチケットで
// 繋ぎ直せ、そのとき先にスクロールバックが再生される。
func TestReattachingReplaysTheScrollbackAndKeepsTheSessionAlive(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 4, Scrollback: 1 << 12})
	id, ticket := fixture.openShell(t)
	process := fixture.starter.last()

	connection, _ := fixture.dial(t, ticket)
	if connection == nil {
		t.Fatal("the stream refused a fresh ticket")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	process.feed("first line\r\n")
	if _, _, err := connection.Read(ctx); err != nil {
		t.Fatal(err)
	}
	// タブを閉じるのに相当する。
	_ = connection.CloseNow()

	process.feed("while detached\r\n")
	session, ok := fixture.registry.Lookup(id)
	if !ok || !session.Live() {
		t.Fatal("closing the socket killed the session")
	}
	// feed は擬似 PTY の読み取り待ちへ出力を渡すだけなので、pump がそれを
	// scrollback へ格納する前に再接続するとライブ出力として次のフレームへ届く。
	// このテストは「切断中に蓄えた scrollback の再生」を検査するため、格納を待つ。
	waitUntil(t, func() bool {
		return strings.Contains(string(session.Snapshot()), "while detached")
	})

	// 新しいチケットで繋ぎ直す。リロードしたページにはチケットが残っていない。
	fresh := fixture.newTicket(t, id)
	again, _ := fixture.dial(t, fresh)
	if again == nil {
		t.Fatal("the reattach was refused")
	}
	kind, payload, err := again.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if kind != websocket.MessageBinary {
		t.Fatalf("the replay was not a binary frame: %v", kind)
	}
	if replayed := string(payload); !strings.Contains(replayed, "first line") ||
		!strings.Contains(replayed, "while detached") {
		t.Fatalf("replay = %q, want everything written while detached", replayed)
	}
}

// newTicket は、すでに開いているセッションへ繋ぎ直すためのチケットを取る。
func (f *terminalFixture) newTicket(t *testing.T, id string) string {
	t.Helper()
	response, body := f.do(t, http.MethodPost, "/api/v1/terminal/sessions/"+id+"/stream", "{}")
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("issue a ticket = %d: %s", response.StatusCode, body)
	}
	var issued api.TerminalStreamTicket
	if err := json.Unmarshal([]byte(body), &issued); err != nil {
		t.Fatal(err)
	}
	return issued.StreamTicket
}

func waitUntil(t *testing.T, condition func() bool) {
	t.Helper()
	// CI の全 package race 実行では、プロセス終了を観測する goroutine が
	// CPU 飽和中に数秒止まることがある。実時間ではなく状態を検査する helper
	// なので、正常系を遅くせずに過負荷時だけ十分待てる上限にする。
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the condition never became true")
}

// 改名は表示だけを変え、応答は一覧を返す。名前が要るのは、同じ相手へ
// 複数本開いたときに行が見分けられなくなるからである。
func TestRenamingASessionChangesTheListedTitle(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 4, Scrollback: 1 << 12})
	id, _ := fixture.openShell(t)

	response, body := fixture.do(t, http.MethodPatch,
		"/api/v1/terminal/sessions/"+id, `{"title":"  ログ監視  "}`)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("rename = %d: %s", response.StatusCode, body)
	}
	var listed api.TerminalSessionList
	if err := json.Unmarshal([]byte(body), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Sessions) != 1 || listed.Sessions[0].Title != "ログ監視" {
		t.Fatalf("the rename response did not carry the trimmed title: %s", body)
	}

	// 拒否された名前は、以前の名前を残す。制御文字はそのまま画面へ出る
	// ので、名前としては受け取らない。
	refusals := []string{`{"title":""}`, `{"title":"esc\u001b[2J"}`, `{"nope":1}`}
	for _, refused := range refusals {
		response, body := fixture.do(t, http.MethodPatch, "/api/v1/terminal/sessions/"+id, refused)
		if response.StatusCode != http.StatusBadRequest {
			t.Errorf("rename %s = %d, want 400: %s", refused, response.StatusCode, body)
		}
	}
	_, body = fixture.do(t, http.MethodGet, "/api/v1/terminal/sessions", "")
	if !strings.Contains(body, "ログ監視") {
		t.Errorf("a refused rename replaced the name: %s", body)
	}

	response, _ = fixture.do(t, http.MethodPatch, "/api/v1/terminal/sessions/absent", `{"title":"x"}`)
	if response.StatusCode != http.StatusNotFound {
		t.Errorf("rename of an unknown session = %d, want 404", response.StatusCode)
	}
}

func TestAgentOSCUpdatesPresentationWithoutExposingNativeReference(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 4, Scrollback: 1 << 12})
	response, body := fixture.do(t, http.MethodPost, "/api/v1/terminal/sessions", `{"kind":"ssh","alias":"osaka"}`)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("open = %d: %s", response.StatusCode, body)
	}
	var opened api.OpenTerminalSessionResponse
	if err := json.Unmarshal([]byte(body), &opened); err != nil {
		t.Fatal(err)
	}
	nativeReference := "thread_private_123"
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"agent":"codex","event":"attention","session":"` + nativeReference + `","name":"API認証の修正","seq":1}`))
	fixture.ssh[0].feed("before\x1b]6973;" + payload[:8])
	fixture.ssh[0].feed(payload[8:] + "\x1b\\after")

	var listed api.TerminalSessionList
	waitUntil(t, func() bool {
		_, current := fixture.do(t, http.MethodGet, "/api/v1/terminal/sessions", "")
		if strings.Contains(current, nativeReference) {
			t.Fatalf("native agent reference escaped into the API: %s", current)
		}
		if json.Unmarshal([]byte(current), &listed) != nil || len(listed.Sessions) != 1 {
			return false
		}
		return listed.Sessions[0].Agent != nil
	})
	session := listed.Sessions[0]
	if session.Title != "API認証の修正" || session.Presentation == nil || session.Presentation.TitleSource != api.Agent {
		t.Fatalf("presentation=%+v title=%q", session.Presentation, session.Title)
	}
	if session.Agent == nil || session.Agent.Kind != api.Codex || session.Agent.State != api.TerminalAgentStateAttention {
		t.Fatalf("agent=%+v", session.Agent)
	}
	core, ok := fixture.registry.Lookup(opened.Session.Id)
	if !ok {
		t.Fatal("opened session disappeared")
	}
	if snapshot := string(core.Snapshot()); snapshot != "beforeafter" {
		t.Fatalf("control payload reached scrollback: %q", snapshot)
	}
}

func TestTitleEndpointPinsAndUnpins(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 4, Scrollback: 1 << 12})
	id, _ := fixture.openShell(t)
	path := "/api/v1/terminal/sessions/" + id + "/title"
	response, body := fixture.do(t, http.MethodPut, path, `{"title":"固定名"}`)
	if response.StatusCode != http.StatusOK || !strings.Contains(body, `"titlePinned":true`) {
		t.Fatalf("pin = %d: %s", response.StatusCode, body)
	}
	response, body = fixture.do(t, http.MethodPut, path, `{"title":null}`)
	if response.StatusCode != http.StatusOK || strings.Contains(body, `"titlePinned":true`) || !strings.Contains(body, `"title":"zsh"`) {
		t.Fatalf("unpin = %d: %s", response.StatusCode, body)
	}
	for _, invalid := range []string{`{}`, `{"title":false}`, `{"title":""}`, `{"unexpected":null}`} {
		response, _ = fixture.do(t, http.MethodPut, path, invalid)
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("invalid %s = %d", invalid, response.StatusCode)
		}
	}
}

func TestAgentResumeReplacesProcessInSamePaneWithoutReceivingACommand(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 4, Scrollback: 1 << 12})
	response, body := fixture.do(t, http.MethodPost, "/api/v1/terminal/sessions", `{"kind":"ssh","alias":"osaka"}`)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("open = %d: %s", response.StatusCode, body)
	}
	var opened api.OpenTerminalSessionResponse
	if json.Unmarshal([]byte(body), &opened) != nil {
		t.Fatal("invalid open response")
	}
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"agent":"claude","event":"ready","session":"session-123","name":"修正作業","seq":1}`))
	fixture.ssh[0].feed("\x1b]6973;" + payload + "\x1b\\")
	var version int
	waitUntil(t, func() bool {
		_, current := fixture.do(t, http.MethodGet, "/api/v1/terminal/sessions", "")
		var listed api.TerminalSessionList
		if json.Unmarshal([]byte(current), &listed) != nil || len(listed.Sessions) != 1 || listed.Sessions[0].Agent == nil {
			return false
		}
		version = listed.Sessions[0].Agent.ObservationVersion
		return true
	})
	request := fmt.Sprintf(`{"observationVersion":%d,"placement":"same-pane"}`, version)
	response, body = fixture.do(t, http.MethodPost, "/api/v1/terminal/sessions/"+opened.Session.Id+"/agent/resume", request)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("resume = %d: %s", response.StatusCode, body)
	}
	var resumed api.OpenTerminalSessionResponse
	if json.Unmarshal([]byte(body), &resumed) != nil || resumed.Session.Id != opened.Session.Id {
		t.Fatalf("resume did not keep pane identity: %s", body)
	}
	fixture.mutex.Lock()
	defer fixture.mutex.Unlock()
	if len(fixture.resumed) != 1 || fixture.resumed[0] != "claude:session-123" {
		t.Fatalf("resume adapter input=%v", fixture.resumed)
	}
}

func TestAgentSamePaneResumeRefusesAfterUserInput(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 4, Scrollback: 1 << 12})
	response, body := fixture.do(t, http.MethodPost, "/api/v1/terminal/sessions", `{"kind":"ssh","alias":"osaka"}`)
	var opened api.OpenTerminalSessionResponse
	if response.StatusCode != http.StatusCreated || json.Unmarshal([]byte(body), &opened) != nil {
		t.Fatalf("open = %d: %s", response.StatusCode, body)
	}
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"agent":"codex","event":"ready","session":"thread-123","seq":1}`))
	fixture.ssh[0].feed("\x1b]6973;" + payload + "\a")
	core, _ := fixture.registry.Lookup(opened.Session.Id)
	waitUntil(t, func() bool { return core.View().Agent != nil })
	if _, err := core.Write([]byte("pwd\r")); err != nil {
		t.Fatal(err)
	}
	version := core.View().Agent.ObservationVersion
	request := fmt.Sprintf(`{"observationVersion":%d,"placement":"same-pane"}`, version)
	response, body = fixture.do(t, http.MethodPost, "/api/v1/terminal/sessions/"+opened.Session.Id+"/agent/resume", request)
	if response.StatusCode != http.StatusConflict || !strings.Contains(body, "agent_resume_same_pane_busy") {
		t.Fatalf("busy resume = %d: %s", response.StatusCode, body)
	}
}
