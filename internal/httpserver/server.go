package httpserver

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v5"

	"sshc/internal/application"
	"sshc/internal/diagnostics"
	"sshc/internal/handoff"
	"sshc/internal/knownhosts"
	"sshc/internal/remotekey"
	"sshc/internal/remotesync"
	"sshc/internal/secret"
	"sshc/internal/selfupdate"
	"sshc/internal/session"
	"sshc/internal/terminal"
)

type Options struct {
	// CLISecret は `sshc <alias>` が提示すべきものである。実行のたびに
	// 発行して state directory に書き込むため、kill されたプロセスが
	// 残した handoff は、誰にも受け付けられない secret を運ぶことになる。
	CLISecret string
	// ConnectWarnings は OpenSSH がそのホストに対して実行するディレクティブを
	// 名指しする。これにより command line は接続中ではなく接続前にそれらを言える。
	ConnectWarnings func(alias string) []string
	// ConnectAliases は、その接続に現れる alias を ProxyJump の手前も含めて
	// 返す。保存済みパスワードを渡す相手をそこに限るために使う。
	ConnectAliases func(alias string) []string
	// Updates はプロジェクトのリリースを調べる。nil の場合、バージョンを
	// 報告するのみで何も提示しない。比較すべきリリースを持たないビルドが
	// すべきことはこれである。
	Updates  *selfupdate.Checker
	Listener net.Listener
	Sessions *session.Manager
	UI       fs.FS
	Version  string
	Owner    handoff.Owner
	// ProtocolVersion は handoff と同じ CLI contract の版である。
	ProtocolVersion int
	Logger          *slog.Logger
	Config          *application.Service
	Keys            KeyService
	// Connect は、alias ひとつ分の対話セッションを開く。合成の根が組み立てる。
	Connect     Connector
	Diagnostics *diagnostics.Service
	KnownHosts  *knownhosts.Service
	RemoteKeys  *remotekey.Service
	// Passwords は保存されたパスワードの vault である。nil の service は
	// すべてのパスワード用ルートを未登録のままに
	// する。これは、それを配線しないテストが当てにしていることである。
	Passwords *secret.Service
	// Sync はワークスペースを object store へ運ぶ。nil の service は
	// すべての sync ルートを未登録のままにする。
	Sync *remotesync.Service
	// AutoSync は、押さなくても進む巡回である。nil でも sync のルートは立つ
	// ——自動同期が無いだけで、手で押す道は残る。
	AutoSync *remotesync.Auto
	// Terminals は埋め込みターミナルのセッションを持つ。nil の registry は
	// セッションのルートと WebSocket を未登録のままにする。これは、それを
	// 配線しないテストが当てにしていることである。
	Terminals *terminal.Registry
	// TerminalStartDirectory は、ローカルシェルが始まる場所を返す。空なら
	// このプロセスの作業ディレクトリを継ぐ——**それは誰も選んでいない場所である。**
	TerminalStartDirectory func() string
	// LoginShell は、PTY の中で起こすローカルシェルを解決する。
	// PATH を見ず、絶対パスかエラーを返す。
	LoginShell func() (string, error)
	// TerminalEnvironment は、端末セッションが継ぐ環境である。これは利用者が
	// 自分で行ったであろう接続なので、検査が使う最小環境ではなく本人の環境を継ぐ。
	TerminalEnvironment func() []string
}

var ErrNonLoopbackListener = errors.New("listener must use 127.0.0.1")

type Server struct {
	listener net.Listener
	http     *http.Server
	url      string
	engine   *echo.Echo

	// baseCancel は、http.Server.BaseContext が配ったリクエスト用 context を
	// 一斉に取り消す。停止を始めた合図が、ハンドラと WebSocket の内側まで届く
	// 唯一の道である。
	baseCancel context.CancelFunc

	// 停止の状態はひとつの錠と合図にまとめてある。要求ごとの入場と退場も同じ
	// 錠を通る——**入場の可否と数え上げが別々に動くと、数えられないまま通った
	// 変更が生まれる。** これは loopback の単一利用者向けなので、その通り道の
	// 競合は問題にならない。
	mutex    sync.Mutex
	waiters  *sync.Cond
	stopping bool
	inFlight int

	begun       bool
	forced      bool
	outstanding int
	joined      []error
	waiting     bool
	waited      bool
	waitErr     error
}

func (s *Server) condition() *sync.Cond {
	if s.waiters == nil {
		s.waiters = sync.NewCond(&s.mutex)
	}
	return s.waiters
}

// BeginStopping は、新しい変更と Upgrade を断り始め、配ったリクエスト用
// context を取り消す。**冪等である。**
//
// 読み取りは断らない。停止の途中でも状態は見られるべきであり、止まるのは
// 状態を変えるものだけである。
func (s *Server) BeginStopping() {
	s.mutex.Lock()
	if s.stopping {
		s.mutex.Unlock()
		return
	}
	s.stopping = true
	s.condition().Broadcast()
	cancel := s.baseCancel
	s.mutex.Unlock()
	if cancel != nil {
		cancel()
	}
}

// BeginShutdown は graceful な停止を頼み、送り出したところで返る。
// **待たない。** 待つのは Wait だけである。
func (s *Server) BeginShutdown() {
	s.mutex.Lock()
	if s.begun {
		s.mutex.Unlock()
		return
	}
	s.begun = true
	s.outstanding++
	s.mutex.Unlock()
	go s.record(func() error { return s.http.Shutdown(context.Background()) })
}

// ForceClose は listener と生きている接続を断つ。**これも待たない。**
//
// graceful が返らないことこそが、これが呼ばれる理由である。まだ blocking して
// いる Shutdown の後ろに並べば、締切は何も起こさないのと同じになる。
func (s *Server) ForceClose() {
	s.mutex.Lock()
	if s.forced {
		s.mutex.Unlock()
		return
	}
	s.forced = true
	s.outstanding++
	s.mutex.Unlock()
	go s.record(s.http.Close)
}

func (s *Server) record(call func() error) {
	err := call()
	s.mutex.Lock()
	if err != nil {
		s.joined = append(s.joined, err)
	}
	s.outstanding--
	s.condition().Broadcast()
	s.mutex.Unlock()
}

// Wait は、唯一の合流点である。
//
// 送り出した graceful/force と、停止より前に入場した変更・Upgrade の退場を
// 待つ。**強制的に接続を閉じたあとも待つ。** これは二度目の猶予ではなく、
// engine lock を手放すことが状態変更と重ならないための壁である。取り消しを
// 無視するハンドラは、2 台目のエンジンを許すよりロックを握らせておく。
func (s *Server) Wait() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.waited {
		return s.waitErr
	}
	if s.waiting {
		for !s.waited {
			s.condition().Wait()
		}
		return s.waitErr
	}
	s.waiting = true
	// ForceClose は Wait の後から仕事を足す。一度ゼロを見たら終わり、では
	// 締切とちょうど同時に最後の要求が退場したときに壁が消える。
	for s.outstanding > 0 || s.inFlight > 0 {
		s.condition().Wait()
	}
	s.waitErr = errors.Join(s.joined...)
	s.waited = true
	s.condition().Broadcast()
	return s.waitErr
}

// admit は、状態を変える要求ひとつを数えるか、断るかを原子的に決める。
func (s *Server) admit() bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.stopping {
		return false
	}
	s.inFlight++
	return true
}

func (s *Server) release() {
	s.mutex.Lock()
	s.inFlight--
	s.condition().Broadcast()
	s.mutex.Unlock()
}

// Route はこの server が登録したルートの 1 つである。
type Route struct {
	Method string
	Path   string
}

// Routes は登録済みの全ルートを登録順に報告する。
//
// hardening suite は自前のリストを持つ代わりにこれを列挙する。これにより
// 後の変更で追加されたルートも、誰も追加を覚えていなくてもトランスポート、
// cache、session、漏洩に関するアサーションを自動的に引き継ぐ。
func (s *Server) Routes() []Route {
	registered := s.engine.Router().Routes()
	routes := make([]Route, 0, len(registered))
	for _, info := range registered {
		routes = append(routes, Route{Method: info.Method, Path: info.Path})
	}
	return routes
}

func New(options Options) (*Server, error) {
	if options.Listener == nil {
		return nil, ErrNonLoopbackListener
	}
	tcpAddress, ok := options.Listener.Addr().(*net.TCPAddr)
	if !ok || len(tcpAddress.IP) != net.IPv4len || tcpAddress.IP[0] != 127 || tcpAddress.IP[1] != 0 || tcpAddress.IP[2] != 0 || tcpAddress.IP[3] != 1 {
		return nil, ErrNonLoopbackListener
	}

	host := net.JoinHostPort("127.0.0.1", strconv.Itoa(tcpAddress.Port))
	e := echo.New()
	if options.Logger != nil {
		e.Logger = options.Logger
	}
	e.Use((Security{
		ExpectedHost:   host,
		ExpectedOrigin: "http://" + host,
		Sessions:       options.Sessions,
		// application はマスターパスワードの向こう側にあるのであって、各画面
		// が個別にそうなのではない。vault なしで組み立てられた server は
		// したがって施錠されたままであり、これは配線し忘れにとって安全な方向である。
		Unlocked: func() bool { return options.Passwords != nil && options.Passwords.Unlocked() },
	}).Middleware)

	server := &Server{listener: options.Listener, engine: e}
	// **入場の門は Security の後、ルートハンドラの前に置く。**
	//
	// 無効な Host や、`/api/` の fetch/origin/session/CSRF に反する要求は、
	// 停止中かどうかに関わらず今までどおり弾かれ、数にも入らない。逆に、
	// ルート側で見ている非 API の資格情報より門の方が先なので、停止が始まった
	// あとは資格情報を確かめる前に 503 が返る。この順序は意図であり、テストで
	// 固定する。
	e.Use(server.stoppingGate)

	handlers := Handlers{Sessions: options.Sessions, Version: options.Version}
	e.POST("/api/v1/session/bootstrap", handlers.Bootstrap)
	e.POST("/api/v1/session/renew", handlers.Renew)
	e.GET("/api/v1/health", handlers.Health)
	if options.Config != nil {
		registerConfigRoutes(e, ConfigHandlers{Service: options.Config, Keys: options.Keys, Secrets: options.Passwords})
		registerConnectionRoutes(e, ConnectionHandlers{Service: options.Config, Keys: options.Keys, Secrets: options.Passwords})
	}

	// 操作を確認するすべてのサブシステムは、自分の evidence resolver を
	// 1 つの registry に提供する。これにより、単一の POST /api/v1/actions
	// エンドポイントが、各 service に踏み込まずにそのどれにでもトークンを発行できる。
	registry := actionRegistry{}
	if options.Keys != nil {
		addKeyActions(registry, options.Keys)
	}
	if options.Diagnostics != nil {
		addDiagnosticsActions(registry, options.Diagnostics)
	}
	if options.KnownHosts != nil {
		addKnownHostsActions(registry, options.KnownHosts)
	}
	actions := ActionHandlers{Sessions: options.Sessions, Kinds: registry}

	if options.Keys != nil {
		registerKeyRoutes(e, KeyHandlers{
			Keys: options.Keys, Config: options.Config, Secrets: options.Passwords,
			Sessions: options.Sessions, Actions: actions,
		})
	}
	if options.Diagnostics != nil {
		registerDiagnosticsRoutes(e, DiagnosticsHandlers{Service: options.Diagnostics, Actions: actions})
	}
	if options.KnownHosts != nil {
		registerKnownHostsRoutes(e, KnownHostsHandlers{Service: options.KnownHosts, Actions: actions})
	}
	if options.RemoteKeys != nil && options.Diagnostics != nil {
		registerRemoteKeyRoutes(e, RemoteKeyHandlers{
			Service: options.RemoteKeys, Diagnostics: options.Diagnostics, Actions: actions,
		})
	}
	// browser と CLI の password change は、同じ remote reseal operation を使う。
	var reseal func(context.Context, string) error
	if options.Sync != nil {
		reseal = func(ctx context.Context, passphrase string) error {
			_, err := options.Sync.Push(ctx, passphrase)
			return err
		}
	}
	vault := newVaultOperations(options.Passwords, reseal)
	if options.Passwords != nil {
		// eligibility チェックは設定グラフと known_hosts を読むため、
		// vault からではなく configuration service から来る。vault は
		// そのどちらについても何も知らない。configuration service がなければ
		// 何もチェックされず、これはこの仕組みができる前に vault がしていた
		// ことであり、vault だけを配線するテストが当てにしていることでもある。
		var eligibility func(string) (application.PasswordEligibility, error)
		if options.Config != nil {
			eligibility = options.Config.PasswordEligibility
		}
		// bucket の最新スナップショットもマスターパスワードで封印されている
		// ため、変更すると再び push する。bucket のないマシンには更新すべき
		// ものがなく、応答はそう伝える。
		var keyHosts func([]string) (map[string][]string, error)
		if options.Config != nil {
			keyHosts = options.Config.KeyHosts
		}
		registerPasswordRoutes(e, PasswordHandlers{
			Service:     options.Passwords,
			vault:       vault,
			KeyHosts:    keyHosts,
			Eligibility: eligibility,
		})
	}
	// `sshc <alias>` は、1 つの接続に必要なものをここに求める。secret は
	// 呼び出し元が state directory から読み出しているはずのものであり、
	// それがなければこのルートはすべてを拒否する。
	registerUpdateRoutes(e, &UpdateHandlers{Current: options.Version, Checker: options.Updates})

	registerConnectRoutes(e, ConnectHandlers{
		Secret:          options.CLISecret,
		Passwords:       options.Passwords,
		vault:           vault,
		Owner:           options.Owner,
		Version:         options.Version,
		ProtocolVersion: options.ProtocolVersion,
		KeyPassphraseTarget: func(alias string) (string, string, string, string, bool, error) {
			if options.Config == nil || options.Keys == nil {
				return "", "", "", "", false, nil
			}
			target, ok, err := options.Config.DirectKeyPassphraseTarget(alias, options.Keys.Inventory)
			return target.RelativePath, target.PromptPath, target.ConfigSnapshot, target.Evidence, ok, err
		},
		Warnings:  options.ConnectWarnings,
		Aliases:   options.ConnectAliases,
		Bootstrap: options.Sessions,
		BaseURL:   "http://" + host,
		Sessions: func() int {
			if options.Terminals == nil {
				return 0
			}
			return liveSessions(options.Terminals.Sessions())
		},
	})
	if options.Sync != nil {
		registerSyncRoutes(e, SyncHandlers{
			Service: options.Sync, Secrets: options.Passwords, Auto: options.AutoSync,
		})
	}
	if options.Terminals != nil {
		registerTerminalRoutes(e, TerminalHandlers{
			Registry:       options.Terminals,
			Tickets:        &terminal.Tickets{},
			Connect:        options.Connect,
			Shell:          options.LoginShell,
			Environment:    options.TerminalEnvironment,
			StartDirectory: options.TerminalStartDirectory,
			// askpass はここに無い。**この経路はもう外部の ssh を起こさない。**
			// パスフレーズは vault から直接読むか、端末で尋ねる。ヘルパーが
			// 残っているのは、CLI と診断がまだ OpenSSH を起こすからである。
			ExpectedOrigin: "http://" + host,
		})
	}
	if len(registry) > 0 {
		registerActionRoutes(e, actions)
	}
	static := echo.WrapHandler(spaHandler(options.UI))
	e.GET("/*", static)
	e.HEAD("/*", static)

	baseCtx, baseCancel := context.WithCancel(context.Background())
	server.baseCancel = baseCancel
	server.url = "http://" + host
	server.http = &http.Server{
		Handler:           e,
		ReadHeaderTimeout: 5 * time.Second,
		// 配ったリクエスト用 context は BeginStopping が一斉に取り消す。
		// これが無いと、停止の合図はハンドラの内側にも WebSocket にも届かない。
		BaseContext: func(net.Listener) context.Context { return baseCtx },
	}
	return server, nil
}

// stoppingGate は、状態を変える要求と Upgrade を、原子的に入場させるか断る。
//
// **ルートの一覧は持たない。** GET と HEAD 以外のすべてと、実際の Upgrade を
// 対象にする。今日の WebSocket は /terminal/stream だが、明日足されるものも
// 誰も何も覚えていなくてもこの規則を継ぐ。
func (s *Server) stoppingGate(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		request := c.Request()
		if request.Method == http.MethodGet && !isUpgrade(request) {
			return next(c)
		}
		if request.Method == http.MethodHead {
			return next(c)
		}
		if !s.admit() {
			return problem(c, http.StatusServiceUnavailable, "server_stopping")
		}
		defer s.release()
		return next(c)
	}
}

func isUpgrade(request *http.Request) bool {
	for _, value := range request.Header.Values("Connection") {
		for _, token := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(token), "upgrade") {
				return request.Header.Get("Upgrade") != ""
			}
		}
	}
	return false
}

func spaHandler(assets fs.FS) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			http.Error(response, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		name := strings.TrimPrefix(path.Clean(request.URL.Path), "/")
		if name == "." || name == "" {
			name = "index.html"
		}
		if !fs.ValidPath(name) {
			http.NotFound(response, request)
			return
		}
		if name == "api" || strings.HasPrefix(name, "api/") || request.URL.Path == "/api" || strings.HasPrefix(request.URL.Path, "/api/") {
			http.NotFound(response, request)
			return
		}

		contents, err := fs.ReadFile(assets, name)
		if err != nil {
			if !acceptsHTML(request.Header.Get("Accept")) {
				http.NotFound(response, request)
				return
			}
			name = "index.html"
			contents, err = fs.ReadFile(assets, name)
			if err != nil {
				http.NotFound(response, request)
				return
			}
		}

		if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
			response.Header().Set("Content-Type", contentType)
		}
		http.ServeContent(response, request, name, time.Time{}, bytes.NewReader(contents))
	})
}

func acceptsHTML(header string) bool {
	for _, value := range strings.Split(header, ",") {
		mediaType, parameters, err := mime.ParseMediaType(strings.TrimSpace(value))
		if err != nil || mediaType != "text/html" {
			continue
		}

		quality := 1.0
		if raw, ok := parameters["q"]; ok {
			quality, err = strconv.ParseFloat(raw, 64)
			if err != nil {
				continue
			}
		}
		if quality > 0 && quality <= 1 {
			return true
		}
	}
	return false
}

func (s *Server) URL() string {
	return s.url
}

// Serve は listener が止まるまで返らない。
//
// **呼び出し側の context を見ない。** 停止を始めてよいのはアプリケーションの
// ライフサイクルだけであり、server が自分で判断すると、承認された順序
// ——入場停止、handoff 削除、二つの graceful 要求——を内側から追い越せてしまう。
func (s *Server) Serve() error {
	return serveResult(s.http.Serve(s.listener))
}

func serveResult(err error) error {
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
