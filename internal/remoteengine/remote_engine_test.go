package remoteengine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"

	"sshc/internal/handoff"
	"sshc/internal/testwait"
)

const fakeSecret = "the-remote-handoff-secret"

// fakeVaultPassword は偽の接続先の Vault のパスワードである。fakeVaultMissing を送ると、
// Vault が無いと断る。
const (
	fakeVaultPassword = "correct horse battery"
	fakeVaultMissing  = "no vault here"
)

// fakeRemoteEngine は、接続先のエンジンの /cli/ と /api/v1/terminal/ の一部を、本物と
// 同じ規則（秘密、cookie、CSRF、Origin、Sec-Fetch-Site）で答える。
type fakeRemoteEngine struct {
	server   *httptest.Server
	sessions atomic.Int32
	// expired が立つと、今の cookie を一度だけ断る（CLI のセッションの期限切れ）。
	expired atomic.Bool
	// stalled が立つと、セッションの一覧に答えない（黙って切れた SSH の接続を真似る）。
	stalled atomic.Bool
	// sessionStalled が立つと、/cli/session に答えない（身元は示すのに止まったエンジン）。
	sessionStalled atomic.Bool
	mutex          sync.Mutex
	cookie         string
	csrf           string
	seen           []string
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
	return handoffDocument(f.server.URL, fakeSecret, version)
}

// handoffDocument は、origin で待ち受け secret を持ち、このマシンと互換のエンジンの handoff である。
func handoffDocument(origin, secret, version string) string {
	body, _ := json.Marshal(handoff.Handoff{
		SchemaVersion: handoff.SchemaVersion, URL: origin, Secret: secret,
		Owner: handoff.OwnerEngine, PID: 4242, Version: version, ProtocolVersion: handoff.ProtocolVersion,
		RunTargetCompatibility: Compatibility,
	})
	return string(body)
}

// withCompatibility は、document の互換の番号を compatibility に替える。nil なら、番号を
// 書かない古い sshc の handoff にする。
func withCompatibility(document string, compatibility *int) string {
	var fields map[string]any
	_ = json.Unmarshal([]byte(document), &fields)
	if compatibility == nil {
		delete(fields, "runTargetCompatibility")
	} else {
		fields["runTargetCompatibility"] = *compatibility
	}
	body, _ := json.Marshal(fields)
	return string(body)
}

func (f *fakeRemoteEngine) serve(w http.ResponseWriter, r *http.Request) {
	f.mutex.Lock()
	f.seen = append(f.seen, r.Method+" "+r.URL.Path)
	f.mutex.Unlock()
	switch r.URL.Path {
	case handoff.ChallengePath:
		w.Header().Set(handoff.ProofHeader, handoff.Prove(fakeSecret, r.Header.Get(handoff.ChallengeHeader)))
		w.WriteHeader(http.StatusNoContent)
	case handoff.SessionPath:
		if r.Header.Get(handoff.HeaderName) != fakeSecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if f.sessionStalled.Load() {
			<-r.Context().Done()
			return
		}
		number := f.sessions.Add(1)
		f.mutex.Lock()
		f.cookie, f.csrf = fmt.Sprintf("cookie-%d", number), fmt.Sprintf("csrf-%d", number)
		cookie, csrf := f.cookie, f.csrf
		f.mutex.Unlock()
		http.SetCookie(w, &http.Cookie{Name: handoff.SessionCookie, Value: cookie})
		_ = json.NewEncoder(w).Encode(map[string]string{"csrfToken": csrf})
	case handoff.StatusPath:
		if r.Header.Get(handoff.HeaderName) != fakeSecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(Status{Version: "v9", Vault: true, Unlocked: true, Sessions: 3})
	case "/api/v1/terminal/sessions":
		if !f.authorized(w, r) {
			return
		}
		if f.stalled.Load() {
			<-r.Context().Done()
			return
		}
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "leaked=1")
		fmt.Fprintf(w, `{"method":%q,"body":%q,"cookies":%q}`, r.Method, string(body), r.Header.Get("Cookie"))
	case "/api/v1/terminal/commands":
		if !f.authorized(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"action":%q}`, r.Header.Get(handoff.ActionHeader))
	case remoteUnlockPath:
		if !f.authorized(w, r) {
			return
		}
		var request struct {
			Passphrase string `json:"passphrase"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		w.Header().Set("Content-Type", "application/json")
		switch request.Passphrase {
		case fakeVaultPassword:
			fmt.Fprint(w, `{"exists":true,"unlocked":true}`)
		case fakeVaultMissing:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"code":"vault_missing"}`)
		default:
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"code":"wrong_passphrase"}`)
		}
	case "/api/v1/terminal/workspaces":
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusConflict)
		fmt.Fprint(w, `{"code":"vault_locked","message":"request rejected"}`)
	case "/api/v1/terminal/shell-profiles":
		w.WriteHeader(http.StatusForbidden)
	case "/api/v1/sftp/media/a-video":
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Set-Cookie", "leaked=1")
		// 保存の名前は接続先が決めたもので、このマシンのメディアの受け口（inline）とは違う。
		w.Header().Set("Content-Disposition", `attachment; filename="clip.html"`)
		// 本物の受け口（httpserver の StreamMedia）と同じく、プレーヤーが求める範囲だけを返す。
		http.ServeContent(w, r, "", time.Time{}, strings.NewReader("the bytes of a video"))
	case "/api/v1/sftp/media/a-page":
		// 乗っ取られた接続先が、メディアの代わりにページを返す。
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<script>steal()</script>")
	case "/api/v1/sftp/media/a-script":
		w.Header().Set("Content-Type", "text/javascript")
		fmt.Fprint(w, "steal()")
	case "/api/v1/sftp/media/a-video-and-a-page":
		w.Header().Add("Content-Type", "video/mp4")
		w.Header().Add("Content-Type", "text/html")
		fmt.Fprint(w, "<script>steal()</script>")
	case "/api/v1/sftp/media/a-video-and-a-page-in-one-line":
		w.Header().Set("Content-Type", "video/mp4, text/html")
		fmt.Fprint(w, "<script>steal()</script>")
	case "/api/v1/sftp/media/an-untyped-page-not-found":
		writeUntypedPage(w, http.StatusNotFound)
	case "/api/v1/sftp/media/a-problem-and-a-page":
		w.Header().Add("Content-Type", "application/problem+json")
		w.Header().Add("Content-Type", "text/html")
		w.WriteHeader(http.StatusNonAuthoritativeInfo)
		fmt.Fprint(w, "<script>steal()</script>")
	case "/api/v1/sftp/media/an-untyped-page":
		writeUntypedPage(w, http.StatusNonAuthoritativeInfo)
	case "/api/v1/answers/untyped":
		writeUntypedPage(w, http.StatusOK)
	case "/api/v1/answers/json-and-a-page":
		w.Header().Add("Content-Type", "application/json")
		w.Header().Add("Content-Type", "text/html")
		fmt.Fprint(w, "<script>steal()</script>")
	case "/api/v1/answers/half-a-type":
		// Go の mime は、/ の無い値も Content-Disposition と同じ読み方で通す。
		w.Header().Set("Content-Type", "html")
		fmt.Fprint(w, "<script>steal()</script>")
	case "/api/v1/answers/any-type":
		w.Header().Set("Content-Type", "*/*")
		fmt.Fprint(w, "<script>steal()</script>")
	case "/api/v1/sftp/media/gone":
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"code":"sftp_not_found","message":"request rejected"}`)
	case "/api/v1/keys":
		// 入口の断り: このマシンのエンジンからの要求の CSRF のトークンが合わない。
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"code":"invalid_csrf","message":"request rejected"}`)
	case "/api/v1/terminal/restore":
		// fetch は Location の無い 3xx を追わず、画面は code で読む。
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusSeeOther)
		fmt.Fprint(w, `{"code":"vault_locked"}`)
	case RemoteStreamPath:
		if r.Header.Get("Origin") != f.origin() || r.Header.Get("Cookie") != "" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		// 接続先が付けたヘッダーがブラウザへ届かないことを確かめるために付ける。
		w.Header().Set("Set-Cookie", handoff.SessionCookie+"=remote; Path=/")
		w.Header().Set("Clear-Site-Data", `"cookies"`)
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

// writeUntypedPage は、種類を付けずにページを返す。偽の接続先の Go の server が本文から種類を判定しないよう、
// Content-Type を nil にする。
func writeUntypedPage(w http.ResponseWriter, status int) {
	w.Header()["Content-Type"] = nil
	w.WriteHeader(status)
	fmt.Fprint(w, "<!doctype html><script>steal()</script>")
}

// authorized は、接続先のエンジンの API の規則（Origin、cookie、CSRF）を確かめる。
// 満たさなければ断りを書いて false を返す。
func (f *fakeRemoteEngine) authorized(w http.ResponseWriter, r *http.Request) bool {
	f.mutex.Lock()
	cookie, csrf := f.cookie, f.csrf
	f.mutex.Unlock()
	sent, _ := r.Cookie(handoff.SessionCookie)
	switch {
	case r.Header.Get("Origin") != f.origin() || r.Header.Get("Sec-Fetch-Site") != "same-origin":
		http.Error(w, "cross_site_request", http.StatusForbidden)
	case sent == nil || sent.Value != cookie || r.Header.Get(handoff.CSRFHeader) != csrf:
		http.Error(w, "invalid_session", http.StatusUnauthorized)
	case f.expired.CompareAndSwap(true, false):
		http.Error(w, "invalid_session", http.StatusUnauthorized)
	default:
		return true
	}
	return false
}

// handoffLink は fakeLink に、handoffCommand への答えを足したものである。
type handoffLink struct {
	*fakeLink
	handoff  string
	exitCode int
}

func (l *handoffLink) Run(ctx context.Context, request RunRequest) (RunResult, error) {
	if request.Command == handoffCommand {
		return RunResult{Stdout: []byte(l.handoff), ExitCode: l.exitCode}, nil
	}
	return l.fakeLink.Run(ctx, request)
}

func newEngineManager(t *testing.T, remote *fakeRemoteEngine, remoteVersion string) *Manager {
	t.Helper()
	return newEngineManagerReading(t, remote, remote.handoffDocument(remoteVersion))
}

// newEngineManagerReading は、接続先で document を handoff として読む Manager を返す。
// このマシンの sshc は v9 である。
func newEngineManagerReading(t *testing.T, remote *fakeRemoteEngine, document string) *Manager {
	t.Helper()
	manager := NewManager(ManagerOptions{
		Store:        newStore(t),
		LocalVersion: "v9",
		Connect: func(context.Context, string) (Link, error) {
			return &handoffLink{fakeLink: newFakeLink(remote.server.URL+"/#x", 0), handoff: document}, nil
		},
	})
	t.Cleanup(manager.Close)
	return manager
}

func forward(t *testing.T, manager *Manager, method, body string) (int, string, http.Header) {
	t.Helper()
	request := httptest.NewRequest(method, "/api/v1/engines/r100/terminal/sessions", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Cookie", "sshc_session=this-machine")
	request.Header.Set(handoff.CSRFHeader, "this-machine-csrf")
	recorder := httptest.NewRecorder()
	if err := manager.ForwardAPI(recorder, request, "r100", "/api/v1/terminal/sessions"); err != nil {
		t.Fatal(err)
	}
	return recorder.Code, recorder.Body.String(), recorder.Header()
}

func TestAnAddedEngineIsReachedWithItsOwnCredentialsAndNotThisMachines(t *testing.T) {
	remote := newFakeRemoteEngine(t)
	manager := newEngineManager(t, remote, "v9")
	if err := manager.AddEngine(context.Background(), "r100"); err != nil {
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
	if err != nil || len(engines) != 1 || engines[0] != "r100" {
		t.Fatalf("engines = %v, %v", engines, err)
	}
}

func TestAnExpiredRemoteSessionIsIssuedAgainOnceAndTheRequestSentAgain(t *testing.T) {
	remote := newFakeRemoteEngine(t)
	manager := newEngineManager(t, remote, "v9")
	if err := manager.AddEngine(context.Background(), "r100"); err != nil {
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

// 一覧は、繋がっているかを答えるだけで、繋ぎには行かない。繋ぎに行くと、画面が実行先の一覧を
// 読むたびに、繋がっていない接続先へ SSH の接続（と認証）を始めてしまう。
func TestAnEngineIsConnectedOnlyWhileItHoldsTheLinkAndTheSessionAndAskingDoesNotConnect(t *testing.T) {
	remote := newFakeRemoteEngine(t)
	connects := 0
	manager := NewManager(ManagerOptions{
		Store:        newStore(t),
		LocalVersion: "v9",
		Connect: func(context.Context, string) (Link, error) {
			connects++
			return &handoffLink{fakeLink: newFakeLink(remote.server.URL+"/#x", 0), handoff: remote.handoffDocument("v9")}, nil
		},
	})
	t.Cleanup(manager.Close)
	if manager.Connected("r100") || connects != 0 {
		t.Fatalf("before adding: connected = %v, connects = %d", manager.Connected("r100"), connects)
	}
	if err := manager.AddEngine(context.Background(), "r100"); err != nil {
		t.Fatal(err)
	}
	if !manager.Connected("r100") {
		t.Fatal("an added engine is not connected")
	}
	if err := manager.RemoveEngine("r100"); err != nil {
		t.Fatal(err)
	}
	if manager.Connected("r100") || connects != 1 {
		t.Fatalf("after removing: connected = %v, connects = %d", manager.Connected("r100"), connects)
	}
}

// このマシンのエンジン自身が送る要求も、取り次ぎと同じ資格で届き、確認のトークンを運ぶ。
// 資格が古ければ受け取り直して一度だけ送り直す。
func TestACallToTheRemoteTerminalAPICarriesTheActionTokenAndRenewsAnExpiredSession(t *testing.T) {
	remote := newFakeRemoteEngine(t)
	manager := newEngineManager(t, remote, "v9")
	if err := manager.AddEngine(context.Background(), "r100"); err != nil {
		t.Fatal(err)
	}
	remote.expired.Store(true)
	answer, err := manager.CallAPI(context.Background(), "r100", APICall{
		Method: http.MethodPost, Path: "/api/v1/terminal/commands", ContentType: "application/json",
		Body: []byte(`{}`), ActionToken: "remote-token",
	})
	if err != nil || answer.Status != http.StatusOK || string(answer.Body) != `{"action":"remote-token"}` {
		t.Fatalf("call = %d %s, %v", answer.Status, answer.Body, err)
	}
	if got := remote.sessions.Load(); got != 2 {
		t.Fatalf("sessions issued = %d, want 2", got)
	}
}

func TestAnEngineOfAnotherVersionWithTheSameCompatibilityIsUsed(t *testing.T) {
	remote := newFakeRemoteEngine(t)
	manager := newEngineManager(t, remote, "v8")
	if err := manager.AddEngine(context.Background(), "r100"); err != nil {
		t.Fatalf("AddEngine with v8 = %v", err)
	}
	if status, err := manager.EngineStatus(context.Background(), "r100"); err != nil || status.Sessions != 3 {
		t.Fatalf("EngineStatus with v8 = %+v, %v", status, err)
	}
	if status, _, _ := forward(t, manager, http.MethodGet, ""); status != http.StatusOK {
		t.Fatalf("forward to v8 = %d, want 200", status)
	}
}

func TestAnIncompatibleEngineIsAddedSoThatItCanBeUpdated(t *testing.T) {
	other := Compatibility + 1
	for _, tc := range []struct {
		name          string
		compatibility *int
	}{
		{"another compatibility", &other},
		{"an older sshc that names none", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			remote := newFakeRemoteEngine(t)
			manager := newEngineManagerReading(t, remote, withCompatibility(remote.handoffDocument("v8"), tc.compatibility))
			if err := manager.AddEngine(context.Background(), "r100"); err != nil {
				t.Fatalf("AddEngine = %v", err)
			}
			if engines, _ := manager.Engines(); !slices.Equal(engines, []string{"r100"}) {
				t.Fatalf("engines = %v, want [r100]", engines)
			}
			var incompatible *IncompatibleEngineError
			if _, err := manager.EngineStatus(context.Background(), "r100"); !errors.As(err, &incompatible) || incompatible.Remote != "v8" || incompatible.Local != "v9" {
				t.Fatalf("EngineStatus = %v, want IncompatibleEngineError naming v8 and v9", err)
			}
			if got := remote.sessions.Load(); got != 0 {
				t.Fatalf("sessions issued = %d, want 0", got)
			}
		})
	}
}

func TestAStoppedEngineIsNotAdded(t *testing.T) {

	stopped := NewManager(ManagerOptions{Store: newStore(t), LocalVersion: "v9", Connect: func(context.Context, string) (Link, error) {
		return &handoffLink{fakeLink: newFakeLink("", 0), exitCode: 1}, nil
	}})
	defer stopped.Close()
	if err := stopped.AddEngine(context.Background(), "r100"); !errors.Is(err, ErrNotRunning) {
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
	if err := manager.AddEngine(context.Background(), "r100"); !errors.Is(err, ErrUnproven) {
		t.Fatalf("AddEngine = %v, want ErrUnproven", err)
	}
}

// 接続先が操作そのものを断った理由（マスターパスワードの間違いなど）は、画面がその code で文を出すので、
// そのまま返す。接続先の入口の断りは、このマシンの画面が自分のセッションの終わりと読まない code にする。
func TestARefusalOfTheOperationItselfReachesTheScreenAndARefusalAtTheEntranceDoesNot(t *testing.T) {
	remote := newFakeRemoteEngine(t)
	manager := newEngineManager(t, remote, "v9")
	if err := manager.AddEngine(context.Background(), "r100"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		remotePath string
		status     int
		code       string
	}{
		{"/api/v1/passwords/unlock", http.StatusForbidden, "wrong_passphrase"},
		{"/api/v1/keys", http.StatusBadGateway, "remote_engine_refused"},
		{"/api/v1/terminal/shell-profiles", http.StatusBadGateway, "remote_engine_refused"},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/api/v1/engines/r100"+strings.TrimPrefix(tc.remotePath, "/api/v1"), nil)
		if err := manager.ForwardAPI(recorder, request, "r100", tc.remotePath); err != nil {
			t.Fatal(err)
		}
		if recorder.Code != tc.status || !strings.Contains(recorder.Body.String(), `"code":"`+tc.code+`"`) {
			t.Errorf("%s = %d %s, want %d %s", tc.remotePath, recorder.Code, recorder.Body.String(), tc.status, tc.code)
		}
	}
}

// servedAnswer は、このマシンの Go の HTTP の server がブラウザへ返した答えである。
type servedAnswer struct {
	status int
	header http.Header
	body   string
}

// serveLikeThisMachine は、forward を Go の HTTP の server に載せて GET し、ブラウザが受け取る答えを返す。
// ResponseRecorder と違い、server は種類の無い答えに、本文から判定した種類を付ける（このマシンのエンジンと同じ）。
// byteRange が空でなければ、プレーヤーと同じく Range を付けて求める。
func serveLikeThisMachine(t *testing.T, forward func(http.ResponseWriter, *http.Request) error, byteRange string) servedAnswer {
	t.Helper()
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := forward(w, r); err != nil {
			t.Errorf("forward = %v", err)
		}
	}))
	defer local.Close()
	request, err := http.NewRequest(http.MethodGet, local.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if byteRange != "" {
		request.Header.Set("Range", byteRange)
	}
	response, err := local.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return servedAnswer{status: response.StatusCode, header: response.Header, body: string(body)}
}

// メディアの経路は、ブラウザが CSRF のトークン無しで開ける。接続先がページやスクリプトを返しても、
// このマシンの画面のオリジンで開かないよう、音声か動画の答えだけを渡す。種類と Content-Disposition は
// このマシンが1つだけ書く。
func TestTheForwardedMediaRouteHandsOverOnlyAudioAndVideo(t *testing.T) {
	remote := newFakeRemoteEngine(t)
	manager := newEngineManager(t, remote, "v9")
	if err := manager.AddEngine(context.Background(), "r100"); err != nil {
		t.Fatal(err)
	}
	mediaRange := func(ticket, byteRange string) servedAnswer {
		return serveLikeThisMachine(t, func(w http.ResponseWriter, r *http.Request) error {
			return manager.ForwardMediaAPI(w, r, "r100", "/api/v1/sftp/media/"+ticket)
		}, byteRange)
	}
	media := func(ticket string) servedAnswer { return mediaRange(ticket, "") }
	video := media("a-video")
	if video.status != http.StatusOK || video.body != "the bytes of a video" || !slices.Equal(video.header.Values("Content-Type"), []string{"video/mp4"}) || video.header.Get("Set-Cookie") != "" {
		t.Fatalf("a video = %d %q, headers %v", video.status, video.body, video.header)
	}
	// 接続先が決めた保存の名前は渡さず、このマシンのメディアの受け口と同じく、その場で再生させる。
	if disposition := video.header.Values("Content-Disposition"); !slices.Equal(disposition, []string{"inline"}) {
		t.Errorf("a video's Content-Disposition = %q, want only inline", disposition)
	}
	// 途中から再生するための範囲は、そのまま届く。
	seek := mediaRange("a-video", "bytes=4-8")
	if seek.status != http.StatusPartialContent || seek.body != "bytes" || seek.header.Get("Content-Range") != "bytes 4-8/20" ||
		!slices.Equal(seek.header.Values("Content-Type"), []string{"video/mp4"}) {
		t.Errorf("a seek = %d %q, headers %v", seek.status, seek.body, seek.header)
	}
	for _, ticket := range []string{
		"a-page", "a-script", "a-video-and-a-page", "a-video-and-a-page-in-one-line",
		"an-untyped-page-not-found", "a-problem-and-a-page", "an-untyped-page",
	} {
		refused := media(ticket)
		if refused.status != http.StatusBadGateway || strings.Contains(refused.body, "steal") || !strings.Contains(refused.body, "remote_engine_refused") ||
			!slices.Equal(refused.header.Values("Content-Type"), []string{"application/problem+json"}) {
			t.Errorf("%s = %d %q (%q)", ticket, refused.status, refused.body, refused.header.Values("Content-Type"))
		}
	}
	// 接続先の断りは、理由ごと届く。
	if gone := media("gone"); gone.status != http.StatusNotFound || !strings.Contains(gone.body, "sftp_not_found") {
		t.Errorf("a ticket the remote does not know = %d %q", gone.status, gone.body)
	}
}

// 取り次ぐ答えの種類は、このマシンが1つだけ書く。種類が無いと、このマシンの Go の server が本文から
// text/html と判定する。2つあると、ブラウザは後ろの種類で読む。/ の無い値と */* は、ブラウザが種類として
// 読まない。どれも、中身の分からない答えにする。
func TestAForwardedAnswerReachesTheBrowserWithExactlyOneContentType(t *testing.T) {
	remote := newFakeRemoteEngine(t)
	manager := newEngineManager(t, remote, "v9")
	if err := manager.AddEngine(context.Background(), "r100"); err != nil {
		t.Fatal(err)
	}
	answers := []struct {
		remotePath string
		want       string
	}{
		{"/api/v1/terminal/commands", "application/json"},
		{"/api/v1/answers/untyped", "application/octet-stream"},
		{"/api/v1/answers/json-and-a-page", "application/octet-stream"},
		{"/api/v1/answers/half-a-type", "application/octet-stream"},
		{"/api/v1/answers/any-type", "application/octet-stream"},
	}
	forwards := map[string]func(http.ResponseWriter, *http.Request, string, string) error{
		"ForwardAPI": manager.ForwardAPI, "ForwardFileAPI": manager.ForwardFileAPI,
	}
	for name, forwardTo := range forwards {
		for _, tc := range answers {
			answer := serveLikeThisMachine(t, func(w http.ResponseWriter, r *http.Request) error {
				return forwardTo(w, r, "r100", tc.remotePath)
			}, "")
			if got := answer.header.Values("Content-Type"); !slices.Equal(got, []string{tc.want}) {
				t.Errorf("%s %s: Content-Type = %q, want only %q", name, tc.remotePath, got, tc.want)
			}
		}
	}
	// このマシンのエンジンが読んでから返す答え（中継のある実行先の取り次ぎ）も、同じ種類になる。
	for _, tc := range answers {
		answer, err := manager.CallAPI(context.Background(), "r100", APICall{Method: http.MethodGet, Path: tc.remotePath})
		if err != nil || answer.ContentType != tc.want {
			t.Errorf("CallAPI %s: Content-Type = %q, %v, want %q", tc.remotePath, answer.ContentType, err, tc.want)
		}
	}
}

// 実行先の操作を確認するトークンは、実行先のエンジンが出して画面が持つ。取り次ぐときに落とすと、
// 実行先は確認の無い操作として断る。
func TestAForwardedRequestCarriesTheConfirmationTokenToTheRemoteEngine(t *testing.T) {
	remote := newFakeRemoteEngine(t)
	manager := newEngineManager(t, remote, "v9")
	if err := manager.AddEngine(context.Background(), "r100"); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/engines/r100/terminal/commands", strings.NewReader("{}"))
	request.Header.Set(handoff.ActionHeader, "token-the-remote-issued")
	recorder := httptest.NewRecorder()
	if err := manager.ForwardAPI(recorder, request, "r100", "/api/v1/terminal/commands"); err != nil {
		t.Fatal(err)
	}
	if recorder.Body.String() != `{"action":"token-the-remote-issued"}` {
		t.Fatalf("the remote engine received %s", recorder.Body.String())
	}
}

// 本文を溜めて取り次ぐ要求も、このマシンの入口が受ける大きさ（共通の上限）までは運ぶ。小さいと、このマシンでは
// 保存できる大きい SSH の設定などが、リモートセッションでだけ断られる。
func TestAForwardedRequestCarriesABodyAsLargeAsThisMachinesEntranceAccepts(t *testing.T) {
	remote := newFakeRemoteEngine(t)
	manager := newEngineManager(t, remote, "v9")
	if err := manager.AddEngine(context.Background(), "r100"); err != nil {
		t.Fatal(err)
	}
	body := strings.Repeat("a", handoff.MaxRequestBodyCeiling)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/engines/r100/terminal/sessions", strings.NewReader(body))
	recorder := httptest.NewRecorder()
	if err := manager.ForwardAPI(recorder, request, "r100", "/api/v1/terminal/sessions"); err != nil {
		t.Fatal(err)
	}
	var received struct {
		Body string `json:"body"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &received); err != nil || len(received.Body) != len(body) {
		t.Fatalf("the remote engine received %d of %d bytes (%v)", len(received.Body), len(body), err)
	}

	// このマシンの入口は、長さを言わない本文を上限で切る。切られた本文は、大きすぎる本文として断る。
	cut := httptest.NewRequest(http.MethodPost, "/api/v1/engines/r100/terminal/sessions", nil)
	cut.Body = http.MaxBytesReader(httptest.NewRecorder(), io.NopCloser(strings.NewReader(body+"a")), handoff.MaxRequestBodyCeiling)
	if err := manager.ForwardAPI(httptest.NewRecorder(), cut, "r100", "/api/v1/terminal/sessions"); !errors.Is(err, ErrRequestTooLarge) {
		t.Fatalf("a body the entrance cut off = %v, want ErrRequestTooLarge", err)
	}
}

func TestOnlyAnAddedEngineIsForwardedToAndRemovingItKeepsTheRemoteRunning(t *testing.T) {
	remote := newFakeRemoteEngine(t)
	manager := newEngineManager(t, remote, "v9")
	request := httptest.NewRequest(http.MethodGet, "/api/v1/engines/r100/terminal/sessions", nil)
	if err := manager.ForwardAPI(httptest.NewRecorder(), request, "r100", "/api/v1/terminal/sessions"); !errors.Is(err, ErrNotAdded) {
		t.Fatalf("forward before adding = %v, want ErrNotAdded", err)
	}
	if err := manager.AddEngine(context.Background(), "r100"); err != nil {
		t.Fatal(err)
	}
	if err := manager.RemoveEngine("r100"); err != nil {
		t.Fatal(err)
	}
	if err := manager.ForwardAPI(httptest.NewRecorder(), request, "r100", "/api/v1/terminal/sessions"); !errors.Is(err, ErrNotAdded) {
		t.Fatalf("forward after removing = %v, want ErrNotAdded", err)
	}
}

func TestTheRemoteStatusIsAskedWithTheHandoffSecret(t *testing.T) {
	remote := newFakeRemoteEngine(t)
	manager := newEngineManager(t, remote, "v9")
	if err := manager.AddEngine(context.Background(), "r100"); err != nil {
		t.Fatal(err)
	}
	status, err := manager.EngineStatus(context.Background(), "r100")
	if err != nil || status.Sessions != 3 || !status.Unlocked {
		t.Fatalf("status = %+v, %v", status, err)
	}
}

func TestATerminalStreamReachesTheRemoteEngineWithItsOriginAndNoCookie(t *testing.T) {
	remote := newFakeRemoteEngine(t)
	manager := newEngineManager(t, remote, "v9")
	if err := manager.AddEngine(context.Background(), "r100"); err != nil {
		t.Fatal(err)
	}
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := manager.ForwardTerminalStream(w, r, "r100"); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
		}
	}))
	defer local.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	header := http.Header{}
	header.Set("Origin", local.URL)
	header.Set("Cookie", "sshc_session=this-machine")
	connection, handshake, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(local.URL, "http")+"/engines/r100/terminal/stream?ticket=t", &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.CloseNow()
	if cookies := handshake.Header.Values("Set-Cookie"); len(cookies) != 0 || handshake.Header.Get("Clear-Site-Data") != "" {
		t.Fatalf("the remote handshake headers reached the browser: Set-Cookie=%q Clear-Site-Data=%q", cookies, handshake.Header.Get("Clear-Site-Data"))
	}
	if err := connection.Write(ctx, websocket.MessageBinary, []byte("ls")); err != nil {
		t.Fatal(err)
	}
	if _, message, err := connection.Read(ctx); err != nil || string(message) != "remote:ls" {
		t.Fatalf("stream answer = %q, %v", message, err)
	}
}

func TestTheRemoteEnginesOwnRefusalsAreNotMistakenForThisMachines(t *testing.T) {
	remote := newFakeRemoteEngine(t)
	manager := newEngineManager(t, remote, "v9")
	if err := manager.AddEngine(context.Background(), "r100"); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		"/api/v1/terminal/workspaces":     "remote_engine_vault_locked",
		"/api/v1/terminal/restore":        "remote_engine_vault_locked",
		"/api/v1/terminal/shell-profiles": "remote_engine_refused",
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/api/v1/engines/r100/terminal/x", nil)
		if err := manager.ForwardAPI(recorder, request, "r100", path); err != nil {
			t.Fatal(err)
		}
		if recorder.Code != http.StatusBadGateway || !strings.Contains(recorder.Body.String(), want) {
			t.Errorf("%s = %d %s, want 502 %s", path, recorder.Code, recorder.Body.String(), want)
		}
	}
}

// controlLink は、handoff を答え、それ以外のコマンドを覚えて、run の結果を返す。
type controlLink struct {
	*handoffLink
	mutex    sync.Mutex
	commands []string
	// run は、handoff 以外のコマンドの結果である。出力は request.Observe へも書く。
	run func(ctx context.Context, request RunRequest) (RunResult, error)
}

func (l *controlLink) Run(ctx context.Context, request RunRequest) (RunResult, error) {
	if request.Command == handoffCommand {
		return l.handoffLink.Run(ctx, request)
	}
	l.mutex.Lock()
	l.commands = append(l.commands, request.Command)
	l.mutex.Unlock()
	return l.run(ctx, request)
}

func (l *controlLink) lastCommand() string {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.commands[len(l.commands)-1]
}

// writeResult は、stdout と stderr を Observe へ書き、結果として返す run である。
func writeResult(result RunResult) func(context.Context, RunRequest) (RunResult, error) {
	return func(_ context.Context, request RunRequest) (RunResult, error) {
		_, _ = request.Observe.Write(result.Stdout)
		_, _ = request.Observe.Write(result.Stderr)
		return result, nil
	}
}

// newControlManager は、link を通して r100 を実行先に足した Manager を返す。
func newControlManager(t *testing.T, remote *fakeRemoteEngine, link *controlLink) *Manager {
	t.Helper()
	link.handoffLink = &handoffLink{fakeLink: newFakeLink("", 0), handoff: remote.handoffDocument("v9")}
	manager := NewManager(ManagerOptions{Store: newStore(t), LocalVersion: "v9", Connect: func(context.Context, string) (Link, error) { return link, nil }})
	t.Cleanup(manager.Close)
	if err := manager.AddEngine(context.Background(), "r100"); err != nil {
		t.Fatal(err)
	}
	return manager
}

// waitForAction は、r100 の操作が終わるまで読み、終わった時点の記録を返す。
func waitForAction(t *testing.T, manager *Manager) ActionLogRead {
	t.Helper()
	var read ActionLogRead
	testwait.Until(t, func() bool {
		var err error
		read, err = manager.ActionLog("r100", 0, actionOutputCapacity)
		return err == nil && read.Done
	})
	return read
}

func TestAnEngineIsUpdatedRestartedOrStoppedWithTheHostsOwnSSHCAndItsCredentialsAreDropped(t *testing.T) {
	remote := newFakeRemoteEngine(t)
	link := &controlLink{run: writeResult(RunResult{Stdout: []byte("sshc: updated\n"), Stderr: []byte("restarted"), ExitCode: 0})}
	manager := newControlManager(t, remote, link)
	for action, arguments := range map[EngineAction]string{ActionUpdate: "update --yes", ActionRestart: "service restart --yes", ActionStop: `"$candidate" stop;`} {
		if _, err := manager.StartAction(context.Background(), "r100", action); err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		read := waitForAction(t, manager)
		if read.Action != action || !read.Succeeded || read.Failure != nil || read.Output != "sshc: updated\nrestarted" {
			t.Fatalf("%s = %+v", action, read)
		}
		if last := link.lastCommand(); !strings.Contains(last, arguments) {
			t.Fatalf("%s ran %q, want it to contain %q", action, last, arguments)
		}
	}
	// 資格を捨てたので、次の取り次ぎは受け取り直す。
	forward(t, manager, http.MethodGet, "")
	if got := remote.sessions.Load(); got != 2 {
		t.Fatalf("sessions issued = %d, want 2", got)
	}
	if _, err := manager.StartAction(context.Background(), "r100", "reboot"); !errors.Is(err, ErrUnknownAction) {
		t.Fatalf("unknown action = %v", err)
	}
}

func TestTheRemoteVaultIsUnlockedWithItsOwnCredentialsAndAWrongPasswordIsToldApart(t *testing.T) {
	remote := newFakeRemoteEngine(t)
	manager := newEngineManager(t, remote, "v9")
	if err := manager.AddEngine(context.Background(), "r100"); err != nil {
		t.Fatal(err)
	}
	remote.expired.Store(true)
	if err := manager.UnlockVault(context.Background(), "r100", fakeVaultPassword); err != nil {
		t.Fatalf("unlock after the remote session expired = %v", err)
	}
	if err := manager.UnlockVault(context.Background(), "r100", "wrong"); !errors.Is(err, ErrWrongPassphrase) {
		t.Fatalf("wrong password = %v", err)
	}
	if err := manager.UnlockVault(context.Background(), "r100", fakeVaultMissing); !errors.Is(err, ErrUnlockRefused) || !strings.Contains(err.Error(), "vault_missing") {
		t.Fatalf("missing vault = %v", err)
	}
	if err := manager.UnlockVault(context.Background(), "gpu-box", fakeVaultPassword); !errors.Is(err, ErrNotAdded) {
		t.Fatalf("an engine that was not added = %v", err)
	}
}

// newManagerBehindRefusingStream は、ターミナルの入出力だけを refuse で断る接続先へ取り次ぐ Manager を返す。
// ほかの要求（handoff、資格）には、偽の接続先が答える。
func newManagerBehindRefusingStream(t *testing.T, refuse func(http.ResponseWriter)) *Manager {
	t.Helper()
	remote := newFakeRemoteEngine(t)
	refusing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != RemoteStreamPath {
			remote.serve(w, r)
			return
		}
		refuse(w)
	}))
	t.Cleanup(refusing.Close)
	// handoff と Origin が、断る側のサーバーを指すようにする。
	remote.server.URL = refusing.URL
	manager := newEngineManager(t, remote, "v9")
	if err := manager.AddEngine(context.Background(), "r100"); err != nil {
		t.Fatal(err)
	}
	return manager
}

func TestARefusedTerminalStreamDoesNotPassTheRemoteCookiesToTheBrowser(t *testing.T) {
	manager := newManagerBehindRefusingStream(t, func(w http.ResponseWriter) {
		w.Header().Add("Set-Cookie", handoff.SessionCookie+"=attacker-chosen; Path=/")
		w.Header().Set("Clear-Site-Data", `"cookies"`)
		w.WriteHeader(http.StatusBadRequest)
	})
	request := httptest.NewRequest(http.MethodGet, "/engines/r100/terminal/stream?ticket=x", nil)
	recorder := httptest.NewRecorder()
	if err := manager.ForwardTerminalStream(recorder, request, "r100"); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("stream = %d, want the remote 400", recorder.Code)
	}
	if cookies := recorder.Header().Values("Set-Cookie"); len(cookies) != 0 || recorder.Header().Get("Clear-Site-Data") != "" {
		t.Fatalf("the remote refusal headers reached the browser: Set-Cookie=%q Clear-Site-Data=%q", cookies, recorder.Header().Get("Clear-Site-Data"))
	}
}

// ターミナルの入出力を接続先が断った答えも、ほかの取り次ぐ答えと同じく、種類はこのマシンが1つだけ書く。
func TestARefusedTerminalStreamReachesTheBrowserWithExactlyOneContentType(t *testing.T) {
	manager := newManagerBehindRefusingStream(t, func(w http.ResponseWriter) { writeUntypedPage(w, http.StatusBadRequest) })
	refused := serveLikeThisMachine(t, func(w http.ResponseWriter, r *http.Request) error {
		return manager.ForwardTerminalStream(w, r, "r100")
	}, "")
	if refused.status != http.StatusBadRequest || !slices.Equal(refused.header.Values("Content-Type"), []string{"application/octet-stream"}) {
		t.Fatalf("a refused stream = %d, Content-Type %q, want the remote 400 with only application/octet-stream", refused.status, refused.header.Values("Content-Type"))
	}
}

// remoteRefusalAnswer は、接続先が返す断りの状態コードと code である。
type remoteRefusalAnswer struct {
	status int
	code   string
}

func TestTheRemoteEnginesVaultAndSessionCodesAreReplacedWhateverTheStatus(t *testing.T) {
	remote := newFakeRemoteEngine(t)
	var answer atomic.Value
	refusing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/terminal/sessions" {
			remote.serve(w, r)
			return
		}
		refusal := answer.Load().(remoteRefusalAnswer)
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(refusal.status)
		fmt.Fprintf(w, `{"code":%q}`, refusal.code)
	}))
	defer refusing.Close()
	remote.server.URL = refusing.URL
	manager := newEngineManager(t, remote, "v9")
	if err := manager.AddEngine(context.Background(), "r100"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		refusal remoteRefusalAnswer
		want    string
	}{
		{remoteRefusalAnswer{http.StatusLocked, "vault_locked"}, "remote_engine_vault_locked"},
		{remoteRefusalAnswer{http.StatusServiceUnavailable, "vault_locked"}, "remote_engine_vault_locked"},
		{remoteRefusalAnswer{http.StatusBadRequest, "invalid_session"}, "remote_engine_refused"},
		{remoteRefusalAnswer{http.StatusConflict, "invalid_csrf"}, "remote_engine_refused"},
		{remoteRefusalAnswer{http.StatusInternalServerError, "session_required"}, "remote_engine_refused"},
	} {
		answer.Store(tc.refusal)
		status, body, _ := forward(t, manager, http.MethodGet, "")
		if status != http.StatusBadGateway || !strings.Contains(body, `"`+tc.want+`"`) {
			t.Errorf("%+v = %d %s, want 502 %s", tc.refusal, status, body, tc.want)
		}
	}
	answer.Store(remoteRefusalAnswer{http.StatusNotFound, "terminal_session_not_found"})
	if status, body, _ := forward(t, manager, http.MethodGet, ""); status != http.StatusNotFound || !strings.Contains(body, "terminal_session_not_found") {
		t.Errorf("another refusal = %d %s, want it passed through", status, body)
	}
}
