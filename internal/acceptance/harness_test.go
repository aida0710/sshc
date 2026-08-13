// Package acceptance は、横断的な hardening suite を保持する。
// production code は含まない: すべてのファイルがテストファイルであり、
// ここにあるものは出荷されるバイナリには一切コンパイルされない。
//
// このパッケージの全テストは、t.TempDir() で隔離された
// ホームディレクトリを構築し、それに対して app.Build 経由で
// production server を起動し、プロセス、terminal、agent の継ぎ目を、プログラムを一切
// 起動しない recorder に置き換える。ここにあるテストは、本物のホームディレクトリ、
// 本物の agent、Terminal、リモートホストのいずれも読まない。
package acceptance_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"sshc/internal/app"
	"sshc/internal/httpserver"
	"sshc/internal/keys"
	"sshc/internal/platform"
	"sshc/internal/terminal"

	"golang.org/x/crypto/ssh"
	"sshc/internal/remotekey"
	"sshc/internal/sshclient"
)

// canaryPassphrase は fixture の private key を保護する。
// reveal の応答を含め、どの response body にもログ行にも決して現れてはならない。
const canaryPassphrase = "canary-passphrase-b0e0a1"

// canaryOutsideContents は ~/.ssh の外のファイルに存在する。
// どの route もそれを返してはならず、leak sweep と path traversal 双方の針である。
const canaryOutsideContents = "canary-outside-workspace-4f21c7\n"

// fixtureCanaries は、テストがレスポンスとログの中に探す文字列を名指しする。
type fixtureCanaries struct {
	Outside        string
	Passphrase     string
	PrivateKeyLine string
	Bootstrap      string
	SessionID      string
	CSRF           string
}

type recordedCommand struct {
	Path      string
	Arguments []string
	Stdin     []byte
	Env       []string
}

// recordingRunner は、アプリケーションが実行するはずのコマンドをすべて
// 記録し、1 つも起動しない。reply を設定すると、特定のテストが必要とする出力を返す。
type recordingRunner struct {
	mutex    sync.Mutex
	commands []recordedCommand
	reply    func(platform.Command) (platform.Output, error)
}

func (r *recordingRunner) RunOutput(_ context.Context, command platform.Command) (platform.Output, error) {
	r.mutex.Lock()
	r.commands = append(r.commands, recordedCommand{
		Path:      command.Path,
		Arguments: append([]string(nil), command.Arguments...),
		Stdin:     append([]byte(nil), command.Stdin...),
		Env:       append([]string(nil), command.Env...),
	})
	reply := r.reply
	r.mutex.Unlock()
	if reply == nil {
		return platform.Output{}, nil
	}
	return reply(command)
}

func (r *recordingRunner) recorded() []recordedCommand {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return append([]recordedCommand(nil), r.commands...)
}

func (r *recordingRunner) reset() {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.commands = nil
}

func (r *recordingRunner) answer(reply func(platform.Command) (platform.Output, error)) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.reply = reply
}

// fixedToolchain は決して実行されない絶対パスを返す。
// なぜなら、それに添えられたランナーが一切プロセスを起動しないからである。
type fixedToolchain struct{}

func (fixedToolchain) KeyGen() (string, error) { return "/usr/bin/ssh-keygen", nil }

// recordingTerminal は、埋め込みターミナルが確保するはずの擬似端末をすべて
// 記録し、1 つも確保しない。このスイートのどのテストも本物の PTY を開かず、
// ssh もシェルも起動しない。
type recordingTerminal struct {
	mutex    sync.Mutex
	commands []terminal.Command
	ptys     []*idlePTY
}

func (t *recordingTerminal) Start(command terminal.Command, _ terminal.Size) (terminal.Process, error) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.commands = append(t.commands, command)
	process := newIdlePTY()
	t.ptys = append(t.ptys, process)
	return process, nil
}

// emit は、いちばん最後に開かれた擬似端末が出力したことにする。
func (t *recordingTerminal) emit(chunk string) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	if len(t.ptys) == 0 {
		return
	}
	t.ptys[len(t.ptys)-1].feed(chunk)
}

// launched は、開かれた端末セッションのプログラムを報告する。
func (t *recordingTerminal) launched() []string {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	programs := make([]string, 0, len(t.commands))
	for _, command := range t.commands {
		programs = append(programs, command.Path)
	}
	return programs
}

func (t *recordingTerminal) opened() []terminal.Command {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return append([]terminal.Command(nil), t.commands...)
}

func (t *recordingTerminal) reset() {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.commands = nil
	t.ptys = nil
}

// idlePTY は、テストが押し込んだものだけを出力し、閉じられるまで生きている
// 擬似端末である。実プロセスは一つも起きない。
type idlePTY struct {
	mutex     sync.Mutex
	pending   [][]byte
	ready     chan struct{}
	closeOnce sync.Once
	done      chan struct{}
}

func newIdlePTY() *idlePTY {
	return &idlePTY{ready: make(chan struct{}, 1), done: make(chan struct{})}
}

// feed は、端末が出力したことにする。
func (p *idlePTY) feed(chunk string) {
	p.mutex.Lock()
	p.pending = append(p.pending, []byte(chunk))
	p.mutex.Unlock()
	select {
	case p.ready <- struct{}{}:
	default:
	}
}

func (p *idlePTY) Read(b []byte) (int, error) {
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

func (p *idlePTY) Write(b []byte) (int, error) { return len(b), nil }
func (p *idlePTY) Resize(terminal.Size) error  { return nil }
func (p *idlePTY) Hangup() error               { p.closeOnce.Do(func() { close(p.done) }); return nil }
func (p *idlePTY) Wait() terminal.ExitInfo     { <-p.done; return terminal.ExitInfo{Signal: "hangup"} }
func (p *idlePTY) Close() error                { p.closeOnce.Do(func() { close(p.done) }); return nil }

// fakeAgent は ssh-agent の代わりを務める。
// このリポジトリのどのテストも、本物のエージェントとは話さない。
type fakeAgent struct{}

func (fakeAgent) Available(context.Context) bool { return false }
func (fakeAgent) List(context.Context) ([]platform.AgentIdentity, error) {
	return nil, platform.ErrAgentUnavailable
}
func (fakeAgent) Add(context.Context, platform.AgentAddRequest) error {
	return platform.ErrAgentUnavailable
}
func (fakeAgent) Remove(context.Context, string) error { return platform.ErrAgentUnavailable }

// silentBrowser は macOS の `open` アダプタを置き換える。テストから本物のブラウザを開けば、
// デスクで動いている何かに生きた bootstrap token を渡すことになる。
type silentBrowser struct{}

func (silentBrowser) Open(context.Context, string) error { return nil }

// testClock はサーバー側の goroutine から読まれ、テスト側から
// 進められるため、時刻は素の field ではなく atomic に保持する。
type testClock struct{ nanoseconds atomic.Int64 }

func newTestClock() *testClock {
	clock := &testClock{}
	clock.nanoseconds.Store(time.Date(2026, time.August, 5, 9, 0, 0, 0, time.UTC).UnixNano())
	return clock
}

func (c *testClock) now() time.Time { return time.Unix(0, c.nanoseconds.Load()).UTC() }

func (c *testClock) advance(step time.Duration) { c.nanoseconds.Add(int64(step)) }

// syncBuffer は、サーバーの goroutine からのログ出力を集める。
type syncBuffer struct {
	mutex  sync.Mutex
	buffer bytes.Buffer
}

func (b *syncBuffer) Write(chunk []byte) (int, error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.buffer.Write(chunk)
}

func (b *syncBuffer) String() string {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.buffer.String()
}

type fixture struct {
	t       testing.TB
	home    string
	root    string
	baseURL string
	host    string
	client  *http.Client
	// anonymous は cookie jar を持たないため、これを通したリクエストは
	// session なしでサーバーに届く。
	anonymous    *http.Client
	server       *httpserver.Server
	runner       *recordingRunner
	terminal     *recordingTerminal
	scanner      *recordingScanner
	clock        *testClock
	logs         *syncBuffer
	canaries     fixtureCanaries
	sessionID    string
	cachedKey    string
	trashCounter atomic.Int64
}

// newFixture は隔離された ~/.ssh を書き出し、それに対して
// production server を起動し、bootstrap token を session と引き換える。
func newFixture(t testing.TB) *fixture {
	t.Helper()
	home := t.TempDir()
	root := filepath.Join(home, ".ssh")
	writeFixtureTree(t, home, root)

	runner := &recordingRunner{}
	terminalStarter := &recordingTerminal{}
	scanner := newRecordingScanner(t)
	clock := newTestClock()
	logs := &syncBuffer{}

	server, bootstrap, err := app.Build(app.Dependencies{
		Home:            home,
		Random:          rand.Reader,
		Browser:         silentBrowser{},
		Listen:          net.Listen,
		UI:              fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>fixture</title><div id=\"root\"></div>")}},
		Logger:          slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
		Runner:          runner,
		Toolchain:       fixedToolchain{},
		TerminalStarter: terminalStarter,
		KeyAgent:        fakeAgent{},
		// このスイートはネットワークへ出ない。ホスト鍵を集める継ぎ目も、
		// 認証を試す継ぎ目も、プロセスの継ぎ目と同じく記録係で置き換える。
		ScanHostKeys: scanner.collect,
		Probe:        scanner.probe,
		RemoteRun:    scanner.remoteRun,
		SessionNow:   clock.now,
	}, "acceptance")
	if err != nil {
		t.Fatalf("app.Build() = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- server.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-served; err != nil {
			t.Errorf("Serve() = %v", err)
		}
	})

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{
		t:         t,
		home:      home,
		root:      root,
		baseURL:   server.URL(),
		host:      strings.TrimPrefix(server.URL(), "http://"),
		client:    &http.Client{Jar: jar, Timeout: 15 * time.Second},
		anonymous: &http.Client{Timeout: 15 * time.Second},
		server:    server,
		runner:    runner,
		terminal:  terminalStarter,
		scanner:   scanner,
		clock:     clock,
		logs:      logs,
		canaries: fixtureCanaries{
			Outside:        strings.TrimSpace(canaryOutsideContents),
			Passphrase:     canaryPassphrase,
			PrivateKeyLine: fixturePrivateKeySecondLine(t, root),
			Bootstrap:      bootstrap,
		},
	}
	f.bootstrapSession(bootstrap)
	// 今やすべての route が master password の背後にあるため、それを設定することは
	// パスワードに関するテストの一部ではなく、アプリケーション起動の一部である。
	f.unlockApplication()
	return f
}

// fixtureMasterPassword は harness が初回起動時に設定する値である。
// これは fixture の値であり、ログ行にもレスポンスにも、平文のどのファイルにも現れない —
// そのこと自体が leak sweep によって検証される。
const fixtureMasterPassword = "a fixture master password"

// unlockAgain は、sweep が閉じた vault を開き直す。すべての route が
// master password の背後にあるため、POST /api/v1/passwords/lock に
// 触れたテストは、それ以降のすべてから自分自身を締め出したことになる。
func (f *fixture) unlockAgain() {
	f.t.Helper()
	body := []byte(`{"passphrase":"` + fixtureMasterPassword + `"}`)
	response := f.do(http.MethodPost, "/api/v1/passwords/unlock", body)
	status := response.StatusCode
	_ = readBody(f.t, response)
	if status != http.StatusOK {
		f.t.Fatalf("unlock the vault again = %d", status)
	}
}

func (f *fixture) unlockApplication() {
	f.t.Helper()
	body := []byte(`{"passphrase":"` + fixtureMasterPassword + `"}`)
	response := f.do(http.MethodPost, "/api/v1/passwords/initialise", body)
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		f.t.Fatalf("initialise the vault = %d", response.StatusCode)
	}
}

// writeFixtureTree は、現実的だが完全に合成された ~/.ssh に加え、
// どの route も決して届いてはならないその外の 1 ファイルを配置する。
func writeFixtureTree(t testing.TB, home, root string) {
	t.Helper()
	mustMkdir(t, root, 0o700)
	mustMkdir(t, filepath.Join(root, "conf.d"), 0o700)
	mustMkdir(t, filepath.Join(home, "private-notes"), 0o700)

	mustWrite(t, filepath.Join(home, "private-notes", "canary.txt"), []byte(canaryOutsideContents), 0o600)
	mustWrite(t, filepath.Join(root, "config"), []byte(""+
		"# Managed by hand since 2019. Do not reformat.\n"+
		"\n"+
		"Include conf.d/*.conf\n"+
		"\n"+
		"Host bastion\n"+
		"\tHostName=203.0.113.10\n"+
		"\tUser    ops\n"+
		"\tPort 2222\n"+
		"\tIdentityFile ~/.ssh/id_ed25519\n"+
		"\n"+
		"Host *\n"+
		"\tServerAliveInterval 30\n"), 0o600)
	mustWrite(t, filepath.Join(root, "conf.d", "10-home.conf"), []byte(""+
		"Host nas\n"+
		"\tHostName 198.51.100.20\n"+
		"\tUnknownFutureDirective some \"quoted value\" 3\n"), 0o600)
	mustWrite(t, filepath.Join(root, "known_hosts"), []byte(""+
		"# a comment the reader must keep\n"+
		"203.0.113.10 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGZpeHR1cmVrZXlmaXh0dXJla2V5Zml4dHVyZWtl fixture\n"), 0o600)

	privateKey, err := keys.GeneratePrivateKey(keys.AlgorithmEd25519, 0, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := keys.EncodePrivateKey(privateKey, "fixture@sshc", []byte(canaryPassphrase))
	if err != nil {
		t.Fatal(err)
	}
	public, err := keys.EncodePublicKey(privateKey, "fixture@sshc")
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "id_ed25519"), encoded, 0o600)
	mustWrite(t, filepath.Join(root, "id_ed25519.pub"), public, 0o644)
}

func mustMkdir(t testing.TB, path string, permission os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(path, permission); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t testing.TB, path string, contents []byte, permission os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, contents, permission); err != nil {
		t.Fatal(err)
	}
}

// fixturePrivateKeySecondLine は、暗号化された private key の
// 内部にある長い base64 行を返す。これは、key material が
// それを運ぶことを許された唯一のレスポンスにとどまったことを証明する針である。
func fixturePrivateKeySecondLine(t testing.TB, root string) string {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(root, "id_ed25519"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(contents)), "\n")
	if len(lines) < 3 {
		t.Fatalf("fixture private key has %d lines", len(lines))
	}
	return lines[1]
}

func (f *fixture) bootstrapSession(bootstrap string) {
	f.t.Helper()
	response := f.do(http.MethodPost, "/api/v1/session/bootstrap", nil, func(request *http.Request) {
		request.Header.Set("X-SSHC-Bootstrap", bootstrap)
	})
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		f.t.Fatalf("bootstrap = %d", response.StatusCode)
	}
	var payload struct {
		CsrfToken string `json:"csrfToken"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		f.t.Fatal(err)
	}
	f.canaries.CSRF = payload.CsrfToken
	for _, cookie := range response.Cookies() {
		if cookie.Name == httpserver.SessionCookie {
			f.sessionID = cookie.Value
			f.canaries.SessionID = cookie.Value
		}
	}
	if f.sessionID == "" {
		f.t.Fatal("bootstrap returned no session cookie")
	}
}

// do は、正しい Host、Origin、Fetch Metadata ヘッダーを付けて 1 つのリクエストを発行する。
// Adjust は最後に適用されるため、テストはそのうちちょうど 1 つだけを誤らせられる。
func (f *fixture) do(method, path string, body []byte, adjust ...func(*http.Request)) *http.Response {
	f.t.Helper()
	return f.doAs(f.t, f.client, method, path, body, adjust...)
}

// doAs は、reporter を明示的に指定した do である。
//
// fuzz target にはこれが要る: newFixture には *testing.F が
// 渡されるが、fuzz function の内側で F に対し Helper や Fatal を
// 呼ぶと panic するため、各実行は代わりに渡された *testing.T を通して報告するほかない。
func (f *fixture) doAs(t testing.TB, client *http.Client, method, path string, body []byte, adjust ...func(*http.Request)) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequest(method, f.baseURL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = f.host
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	// token は write だけでなく read にも伴う。なぜなら cookie は
	// ポートに scope されないが token はされるからである: 127.0.0.1 上の
	// 別のサーバーは cookie を受け取っても token は決して受け取らない。
	request.Header.Set(httpserver.CSRFHeader, f.canaries.CSRF)
	if method != http.MethodGet && method != http.MethodHead {
		request.Header.Set("Origin", f.baseURL)
		request.Header.Set("Content-Type", "application/json")
	}
	for _, apply := range adjust {
		apply(request)
	}
	response, err := client.Do(request)
	if err == nil && path == "/api/v1/session/renew" && response.StatusCode == http.StatusOK {
		// Renewing は token を rotate させ、harness は frontend と
		// 全く同じようにそれを追わねばならない。route sweep はこれを含む
		// 全 route を呼ぶため、これがなければ同じテスト内でこの後の
		// すべてのリクエストが、session がもはや知らない token を運ぶ
		// ことになる — read が token を必要としない間は見えなかった問題である。
		body, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr == nil {
			var renewed struct {
				CsrfToken string `json:"csrfToken"`
			}
			if json.Unmarshal(body, &renewed) == nil && renewed.CsrfToken != "" {
				f.canaries.CSRF = renewed.CsrfToken
			}
		}
		response.Body = io.NopCloser(bytes.NewReader(body))
	}
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return response
}

// doAnonymous は do と同じリクエストを発行するが、cookie jar を
// 持たない client を通すため、session cookie は正真正銘存在しない。
//
// adjust function の内側で Cookie ヘッダーを削除しても効かない:
// http.Client は、呼び出し元がリクエストの構築を終えた後で jar の
// cookie を付け足すため、ヘッダーは再び現れてしまい、session 要件を
// 証明するつもりのテストが、黙って cookie を送ってしまうことになる。
func (f *fixture) doAnonymous(method, path string, body []byte, adjust ...func(*http.Request)) *http.Response {
	f.t.Helper()
	return f.doAs(f.t, f.anonymous, method, path, body, adjust...)
}

// readBody はレスポンスを読み干して閉じ、その body をテキストとして返す。
func readBody(t testing.TB, response *http.Response) string {
	t.Helper()
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// apiRoutes は /api/ 配下の登録済み route をすべて返す。
// ただし static の catch-all は除く。
func (f *fixture) apiRoutes() []httpserver.Route {
	var routes []httpserver.Route
	for _, route := range f.server.Routes() {
		if strings.HasPrefix(route.Path, "/api/") {
			routes = append(routes, route)
		}
	}
	if len(routes) == 0 {
		f.t.Fatal("the server registered no API route; the harness is not wiring the services")
	}
	return routes
}

// concretePath は各 Echo path parameter に使える値を代入するため、
// /api/v1/keys/:keyId のような route も汎用の sweep からリクエストできる。
func (f *fixture) concretePath(path string) string {
	segments := strings.Split(path, "/")
	for index, segment := range segments {
		if !strings.HasPrefix(segment, ":") {
			continue
		}
		switch segment {
		case ":keyId":
			segments[index] = f.keyID()
		default:
			segments[index] = "acceptance-placeholder"
		}
	}
	return strings.Join(segments, "/")
}

// keyID はインベントリを一度読み、fixture の private key の
// identifier を返す。
func (f *fixture) keyID() string {
	f.t.Helper()
	if f.cachedKey != "" {
		return f.cachedKey
	}
	response := f.do(http.MethodGet, "/api/v1/keys", nil)
	status := response.StatusCode
	body := readBody(f.t, response)
	if status != http.StatusOK {
		f.t.Fatalf("the key inventory answered %d: %s", status, body)
	}
	var payload struct {
		Items []struct {
			ID           string `json:"id"`
			RelativePath string `json:"relativePath"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		f.t.Fatalf("inventory did not decode: %v", err)
	}
	for _, item := range payload.Items {
		if item.RelativePath == "id_ed25519" {
			f.cachedKey = item.ID
			return item.ID
		}
	}
	f.t.Fatal("the fixture private key is not in the inventory")
	return ""
}

// knownHostsPath は、サーバー自身が known_hosts として報告するパスを返す。
//
// これは filepath.Join(f.root, "known_hosts") ではない: known_hosts
// 変更に対する token は、ワークスペース自身が綴るそのパスに紐づいており、
// macOS では t.TempDir() が返すのは /var の symlink で、解決形は /private/var である。
func (f *fixture) knownHostsPath() string {
	f.t.Helper()
	body := readBody(f.t, f.do(http.MethodGet, "/api/v1/known-hosts", nil))
	var payload struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil || payload.Path == "" {
		f.t.Fatalf("the known_hosts listing did not report a path: %s", body)
	}
	return payload.Path
}

// newTrashEntry は使い捨ての key を生成して trash に入れ、
// trash entry の identifier を返す。
//
// 完全削除の確認は entry から導かれる evidence に
// 紐づくため、存在しない entry には token を発行できない。
// 呼び出し元はそれぞれ自分の entry を得るので、拒否のケースは
// 1 つを消費する positive control から独立していられる。
func (f *fixture) newTrashEntry(t testing.TB) string {
	t.Helper()
	name := "acceptance-" + strconv.Itoa(int(f.trashCounter.Add(1)))
	generated := f.do(http.MethodPost, "/api/v1/keys", mustJSON(t, map[string]any{
		"algorithm": "ed25519", "fileName": name, "comment": "acceptance",
		"passphrase": "", "unencrypted": true,
	}))
	generatedBody := readBody(t, generated)
	if generated.StatusCode != http.StatusCreated {
		t.Fatalf("generate %s = %d: %s", name, generated.StatusCode, generatedBody)
	}
	var key struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(generatedBody), &key); err != nil || key.ID == "" {
		t.Fatalf("generate did not report an id: %s", generatedBody)
	}

	trashed := f.do(http.MethodPost, "/api/v1/keys/"+key.ID+"/trash", nil)
	trashedBody := readBody(t, trashed)
	if trashed.StatusCode != http.StatusOK {
		t.Fatalf("trash %s = %d: %s", name, trashed.StatusCode, trashedBody)
	}
	var entry struct {
		EntryID string `json:"entryId"`
	}
	if err := json.Unmarshal([]byte(trashedBody), &entry); err != nil || entry.EntryID == "" {
		t.Fatalf("trash did not report an entry: %s", trashedBody)
	}
	return entry.EntryID
}

func (f *fixture) logText() string { return f.logs.String() }

func (f *fixture) read(relative string) []byte {
	f.t.Helper()
	contents, err := os.ReadFile(filepath.Join(f.root, relative))
	if err != nil {
		f.t.Fatal(err)
	}
	return contents
}

// actionToken は、汎用の確認操作について frontend と全く同じように、
// 実行中のサーバーへ token を要求する。表示内容が kind と target だけでは
// 表せない公開鍵登録は、下の remoteKeyPlanToken を使う。
func (f *fixture) actionToken(t testing.TB, kind, target string) string {
	t.Helper()
	token := f.tryActionToken(kind, target)
	if token == "" {
		t.Fatalf("POST /api/v1/actions issued no %q token for %q", kind, target)
	}
	return token
}

// tryActionToken は target が許容できるとき token を発行し、
// そうでなければ空文字列を返すため、敵対的な target でもテストを中断させない。
func (f *fixture) tryActionToken(kind, target string) string {
	response := f.do(http.MethodPost, "/api/v1/actions", mustJSON(f.t, map[string]any{
		"kind": kind, "target": target,
	}))
	status := response.StatusCode
	body := readBody(f.t, response)
	if status != http.StatusOK && status != http.StatusCreated {
		return ""
	}
	var payload struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return ""
	}
	return payload.Token
}

// remoteKeyPlanToken は、公開鍵登録の確認画面と同じ request から、計画に
// 同梱された token を取り出す。この操作だけは接続名だけでなく、表示された
// 接続先・ユーザー・鍵・設置先・実行方式の全体へ確認が結び付く。
func (f *fixture) remoteKeyPlanToken(t testing.TB, alias string) string {
	t.Helper()
	response := f.do(http.MethodPost, "/api/v1/remote-keys/plan", mustJSON(t, map[string]any{
		"alias": alias, "keyPath": "id_ed25519.pub",
		"publicKey": string(bytes.TrimSpace(f.read("id_ed25519.pub"))),
	}))
	status := response.StatusCode
	body := readBody(t, response)
	if status != http.StatusOK {
		t.Fatalf("POST /api/v1/remote-keys/plan = %d: %s", status, body)
	}
	var payload struct {
		ActionToken string `json:"actionToken"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil || payload.ActionToken == "" {
		t.Fatalf("remote-key plan issued no token: %s", body)
	}
	return payload.ActionToken
}

// withAction は、すべての guarded route が期待する形で確認を届ける。
func withAction(token string) func(*http.Request) {
	return func(request *http.Request) {
		if token != "" {
			request.Header.Set("X-SSHC-Action", token)
		}
	}
}

func mustJSON(t testing.TB, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func TestHarnessStartsTheProductionServerAgainstAnIsolatedHome(t *testing.T) {
	f := newFixture(t)
	response := f.do(http.MethodGet, "/api/v1/health", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("health = %d", response.StatusCode)
	}
	readBody(t, response)
	if len(f.apiRoutes()) < 2 {
		t.Fatalf("routes = %#v", f.apiRoutes())
	}
	if _, err := os.Stat(filepath.Join(f.home, ".ssh", "config")); err != nil {
		t.Fatalf("fixture config missing: %v", err)
	}
}

// recordingScanner は、ホスト鍵を集める継ぎ目である。
//
// **このスイートはネットワークへ出ない。** 本物の握手を見るのは
// internal/sshclient の側であり、ここが確かめるのは、確認の無い要求が
// この継ぎ目に届かないことである。
type recordingScanner struct {
	// hostKey は、この記録係が「そのアドレスにあった」と答える鍵である。
	//
	// **本物の鍵でなければならない。** known_hosts のフィクスチャに書いてある
	// 合成の行は wire format として短く、公開鍵として読み戻せない。
	hostKey ssh.PublicKey

	mutex     sync.Mutex
	addresses []string
	probed    []string
	ran       []remoteCall
	// answer は、認証の継ぎ目が返す答えである。既定は「届かなかった」。
	answer func() (sshclient.Probe, error)
}

func (s *recordingScanner) collect(
	_ context.Context, address string, _ time.Duration,
) ([]ssh.PublicKey, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.addresses = append(s.addresses, address)
	return []ssh.PublicKey{s.hostKey}, nil
}

// reached は、この継ぎ目に届いた宛先である。
func (s *recordingScanner) reached() []string {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return append([]string(nil), s.addresses...)
}

func (s *recordingScanner) reset() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.addresses, s.probed, s.ran = nil, nil, nil
}

// probe は、認証テストの継ぎ目である。**このスイートは認証しない。**
func (s *recordingScanner) probe(_ context.Context, alias string) (sshclient.Probe, error) {
	s.mutex.Lock()
	answer := s.answer
	s.probed = append(s.probed, alias)
	s.mutex.Unlock()
	if answer != nil {
		return answer()
	}
	return sshclient.Probe{}, errors.New("no route to host")
}

// answers は、以降の認証がどう答えるかを決める。
func (s *recordingScanner) answers(answer func() (sshclient.Probe, error)) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.answer = answer
}

// authenticated は、この継ぎ目に届いた alias である。
func (s *recordingScanner) authenticated() []string {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return append([]string(nil), s.probed...)
}

// remoteRun は、リモートで 1 本のコマンドを走らせる継ぎ目である。
func (s *recordingScanner) remoteRun(
	_ context.Context, target sshclient.Target, command string, stdin []byte,
) (sshclient.Output, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.ran = append(s.ran, remoteCall{alias: target.Alias, command: command, stdin: string(stdin)})
	if command == remotekey.ProbeCommand {
		return sshclient.Output{Stdout: []byte(remotekey.ProbeMarker + "\n")}, nil
	}
	return sshclient.Output{Stdout: []byte("sshc: added\n")}, nil
}

// remoteCall は、リモートで走らせた 1 本のコマンドである。
type remoteCall struct {
	alias   string
	command string
	stdin   string
}

// remoted は、リモート実行の継ぎ目に届いたものである。
func (s *recordingScanner) remoted() []remoteCall {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return append([]remoteCall(nil), s.ran...)
}

func newRecordingScanner(t testing.TB) *recordingScanner {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	return &recordingScanner{hostKey: signer.PublicKey()}
}
