package httpserver

import (
	"context"
	"encoding/json"
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

type scriptedStarter struct {
	mutex     sync.Mutex
	processes []*scriptedPTY
	commands  []terminal.Command
}

func (s *scriptedStarter) Start(command terminal.Command, _ terminal.Size) (terminal.Process, error) {
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
		SSH:            func() (string, error) { return "/usr/bin/ssh", nil },
		ExpectedOrigin: fixture.origin,
	})
	server.Config.Handler = engine
	server.Start()
	fixture.server = server
	t.Cleanup(func() {
		fixture.server.Close()
		registry.Shutdown()
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
	if listed.MaxSessions != 4 {
		t.Fatalf("maxSessions = %d", listed.MaxSessions)
	}

	// ローカルシェルはログインシェルとして起こす。先頭のハイフンがそれを伝える
	// 唯一の手段である。
	opened := fixture.starter.opened()
	if len(opened) != 1 || opened[0].Argv0 == "" || opened[0].Argv0[0] != '-' {
		t.Fatalf("command = %#v", opened)
	}
}

// 上限に達した状態で開こうとした要求は拒否する。黙って古いセッションを
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

func TestClosingHangsUpAndThenForgets(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 4, Scrollback: 1 << 12})
	id, _ := fixture.openShell(t)

	response, body := fixture.do(t, http.MethodDelete, "/api/v1/terminal/sessions/"+id, "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("close = %d: %s", response.StatusCode, body)
	}
	// 終了は非同期に観測される。終了済みとして一覧に残るのを待つ。
	waitUntil(t, func() bool {
		session, ok := fixture.registry.Lookup(id)
		return ok && session.Exit() != nil
	})

	// 二度目が一覧から消す。終了済みを残すのは、最後の出力を読めるようにするためで、
	// 消すのは人が明示的にそう言ったときだけである。
	if response, body := fixture.do(t, http.MethodDelete, "/api/v1/terminal/sessions/"+id, ""); response.StatusCode != http.StatusOK {
		t.Fatalf("second close = %d: %s", response.StatusCode, body)
	}
	if _, ok := fixture.registry.Lookup(id); ok {
		t.Fatal("the second close left the session in the list")
	}
	if response, _ := fixture.do(t, http.MethodDelete, "/api/v1/terminal/sessions/"+id, ""); response.StatusCode != http.StatusNotFound {
		t.Fatalf("closing a missing session = %d, want 404", response.StatusCode)
	}
}

// チケットは使い捨てである。二度目も、他人が考えた値も、アップグレードせずに
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
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the condition never became true")
}
