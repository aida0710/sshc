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
	"sshc/internal/browserauth"
	"sshc/internal/diagnostics"
	"sshc/internal/handoff"
	"sshc/internal/knownhosts"
	"sshc/internal/platform"
	"sshc/internal/recent"
	"sshc/internal/remotekey"
	"sshc/internal/remotesync"
	"sshc/internal/secret"
	"sshc/internal/selfupdate"
	"sshc/internal/session"
	sshcSFTP "sshc/internal/sftp"
	"sshc/internal/snippets"
	"sshc/internal/terminal"
	"sshc/internal/workspace"
)

type Options struct {
	// CLISecret は `sshc ssh <alias>` が提示すべきものである。実行のたびに
	// 発行して state directory に書き込むため、kill されたプロセスが
	// 残した handoff は、誰にも受け付けられない secret を運ぶことになる。
	CLISecret string
	// ConnectWarnings は OpenSSH がそのホストに対して実行するディレクティブを
	// 指定する。これにより command line は接続中ではなく接続前にそれらを言える。
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
	// BrowserAuth keeps only hashes of browser enrolment capabilities. It is
	// device-local state and lets a fixed-origin bookmark recover after restart.
	BrowserAuth *browserauth.Store
	UI          fs.FS
	Version     string
	Owner       handoff.Owner
	// StopEngine は engine の停止を要求する。nil の場合、停止 API は未実装として応答する。
	StopEngine func()
	// ProtocolVersion は handoff と同じ CLI contract の版である。
	ProtocolVersion int
	Logger          *slog.Logger
	Config          *application.Service
	Keys            KeyService
	// Connect は、alias ひとつ分の対話セッションを開く。合成の根が組み立てる。
	Connect           Connector
	ConnectAgent      AgentConnector
	ConnectionBinding func(alias string) (string, error)
	Diagnostics       *diagnostics.Service
	KnownHosts        *knownhosts.Service
	RemoteKeys        *remotekey.Service
	// Recent は、この端末で成功した接続を現在の設定へ解決する。
	Recent     *recent.Service
	SFTP       *sshcSFTP.Service
	Workspaces *workspace.Service
	Snippets   *snippets.Service
	// Passwords は保存されたパスワードの vault である。nil の service は
	// すべてのパスワード用ルートを未登録のままに
	// する。これは、それを配線しないテストが当てにしていることである。
	Passwords *secret.Service
	// Sync はワークスペースを object store へ運ぶ。nil の service は
	// すべての sync ルートを未登録のままにする。
	Sync *remotesync.Service
	// AutoSync は自動同期の巡回処理。nil でも手動同期 API は登録する。
	AutoSync *remotesync.Auto
	// Terminals は埋め込みターミナルのセッションを持つ。nil の registry は
	// セッションのルートと WebSocket を未登録のままにする。これは、それを
	// 配線しないテストが当てにしていることである。
	Terminals *terminal.Registry
	// ConnectionOpened は、SSHセッションとstream ticketの両方を作れたあとに呼ぶ。
	ConnectionOpened func(alias string)
	// TerminalStartDirectory は、ローカルシェルが始まる場所を返す。空なら
	// このプロセスの作業ディレクトリを継ぐ。それは誰も選んでいない場所である。
	TerminalStartDirectory func() string
	// LoginShell は、PTY の中で起動するローカルシェルを解決する。
	// PATH を見ず、絶対パスかエラーを返す。
	LoginShell func() (string, error)
	// LocalShellProfiles are detected executable/argv pairs; the browser receives
	// IDs only and can never provide an arbitrary command line.
	LocalShellProfiles        func() []platform.ShellProfile
	TerminalLocalShellProfile func() string
	// TerminalEnvironment は、端末セッションが継ぐ環境である。これは利用者が
	// 自分で行ったであろう接続なので、検査が使う最小環境ではなくユーザー本人の環境を継ぐ。
	TerminalEnvironment func() []string
}

var ErrNonLoopbackListener = errors.New("listener must use 127.0.0.1")

type Server struct {
	listener  net.Listener
	http      *http.Server
	url       string
	engine    *echo.Echo
	transfers interface{ Close() error }

	// baseCancel は、停止開始時に全リクエストと WebSocket の context を取り消す。
	baseCancel context.CancelFunc

	// 停止状態と通知は同じ mutex で保護する。要求の開始と終了も同じ
	// mutex を使い、受付可否と実行中件数を一貫して更新する。
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

// BeginStopping は、新しい変更、Upgrade、SFTP data-plane 要求を断り始め、
// 配ったリクエスト用 context を取り消す。冪等である。
//
// 通常の読み取りは断らない。停止の途中でも状態は見られるべきである。
// ただしSFTPのGET／HEADはSSH transport、plaintext spool、transfer jobを所有する
// data-plane操作なので、終了資源との競合を避けるため変更要求と同じ境界で止める。
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
// 待たない。待つのは Wait だけである。
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

// ForceClose は listener と実行中接続を閉じる。完了は Wait で待つ。
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
// 待つ。強制的に接続を閉じたあとも待つ。これは二度目の猶予ではなく、
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
	transfers := s.transfers
	if transfers != nil {
		// A remote Close may block in transport code. Do not hold the server
		// condition mutex across it: a concurrent force request and other Wait
		// callers still need to observe the in-progress join.
		s.mutex.Unlock()
		transferErr := transfers.Close()
		s.mutex.Lock()
		if transferErr != nil {
			s.joined = append(s.joined, transferErr)
		}
		// ForceClose may have registered work while the mutex was released for
		// transport cleanup. Preserve the original Wait contract and join it too.
		for s.outstanding > 0 || s.inFlight > 0 {
			s.condition().Wait()
		}
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
		// Passwords が未設定の場合も安全側としてロック済みと扱う。
		Unlocked: func() bool { return options.Passwords != nil && options.Passwords.Unlocked() },
	}).Middleware)

	baseCtx, baseCancel := context.WithCancel(context.Background())
	server := &Server{listener: options.Listener, engine: e, baseCancel: baseCancel}
	// stoppingGate は Security の後、ルートハンドラの前に置く。
	//
	// 無効な Host や、`/api/` の fetch/origin/session/CSRF に反する要求は、
	// 停止中かどうかに関わらず今までどおり弾かれ、数にも入らない。逆に、
	// ルート側で検証する非 API の資格情報より stoppingGate が先に動くため、停止開始後は
	// 資格情報を確かめる前に 503 が返る。この順序はテストで
	// 固定する。
	e.Use(server.stoppingGate)

	handlers := Handlers{Sessions: options.Sessions, BrowserAuth: options.BrowserAuth, Version: options.Version}
	e.POST("/api/v1/session/bootstrap", handlers.Bootstrap)
	e.POST("/api/v1/session/recover", handlers.Recover)
	e.POST("/api/v1/session/renew", handlers.Renew)
	e.GET("/api/v1/health", handlers.Health)
	if options.Config != nil {
		registerConfigRoutes(e, ConfigHandlers{Service: options.Config, Keys: options.Keys, Secrets: options.Passwords})
		registerConnectionRoutes(e, ConnectionHandlers{
			Service: options.Config, Keys: options.Keys, Secrets: options.Passwords, Recent: options.Recent,
		})
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
	if options.SFTP != nil {
		addSFTPActions(registry, options.SFTP)
	}
	if options.Sync != nil {
		addSyncActions(registry, options.Sync)
	}
	if options.Passwords != nil {
		addCredentialActions(registry, options.Passwords)
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
	if options.SFTP != nil {
		transfers := sshcSFTP.NewTransferManager(options.SFTP)
		server.transfers = transfers
		registerSFTPRoutes(e, SFTPHandlers{
			Service: options.SFTP, Transfers: transfers, Actions: actions,
		})
	}
	if options.Workspaces != nil {
		registerWorkspaceRoutes(e, WorkspaceHandlers{Service: options.Workspaces})
	}
	if options.Snippets != nil {
		registerSnippetRoutes(e, SnippetHandlers{Service: options.Snippets, Actions: actions, BaseContext: baseCtx})
	}
	vault := newVaultOperations(options.Passwords)
	if options.Passwords != nil {
		// Eligibility and authentication-destination binding come from the
		// configuration graph. A missing configuration service leaves password
		// writes fail-closed in PasswordHandlers.
		var eligibility func(string) (application.PasswordEligibility, error)
		var passwordBinding func(string) (string, error)
		if options.Config != nil {
			eligibility = options.Config.PasswordEligibility
			passwordBinding = options.Config.PasswordBinding
		}
		var keyHosts func([]string) (map[string][]string, error)
		if options.Config != nil {
			keyHosts = options.Config.KeyHosts
		}
		registerPasswordRoutes(e, PasswordHandlers{
			Service:     options.Passwords,
			vault:       vault,
			Actions:     actions,
			KeyHosts:    keyHosts,
			Eligibility: eligibility,
			Binding:     passwordBinding,
		})
	}
	// `sshc ssh <alias>` は、1 つの接続に必要なものをここに求める。secret は
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
		StopEngine:      options.StopEngine,
		WorkspaceKeys: func(alias string) ([]string, error) {
			if options.Config == nil || options.Keys == nil {
				return nil, nil
			}
			return options.Config.UnlockableWorkspaceKeys(alias, options.Keys.Inventory)
		},
		Warnings: options.ConnectWarnings,
		Aliases:  options.ConnectAliases,
		PasswordBinding: func(alias string) (string, error) {
			if options.Config == nil {
				return "", errors.New("configuration unavailable")
			}
			return options.Config.PasswordBinding(alias)
		},
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
			Actions: actions,
		})
	}
	if options.Terminals != nil {
		registerTerminalRoutes(e, TerminalHandlers{
			Registry:            options.Terminals,
			Tickets:             &terminal.Tickets{},
			Snippets:            options.Snippets,
			Actions:             actions,
			Connect:             options.Connect,
			ConnectAgent:        options.ConnectAgent,
			ConnectionBinding:   options.ConnectionBinding,
			Shell:               options.LoginShell,
			ShellProfiles:       options.LocalShellProfiles,
			DefaultShellProfile: options.TerminalLocalShellProfile,
			Environment:         options.TerminalEnvironment,
			StartDirectory:      options.TerminalStartDirectory,
			Connected:           options.ConnectionOpened,
			Startup: func(alias string) (string, bool) {
				if options.Snippets == nil {
					return "", false
				}
				prepared, err := options.Snippets.PrepareStartupCommand(alias)
				if err != nil {
					return "", false
				}
				return prepared.Command, true
			},
			// askpass はここに無い。この経路はもう外部の ssh を起動しない。
			// パスフレーズは vault から直接読むか、端末で尋ねる。ヘルパーが
			// 残っているのは、CLI と診断がまだ OpenSSH を起動するからである。
			ExpectedOrigin: "http://" + host,
		})
	}
	if len(registry) > 0 {
		registerActionRoutes(e, actions)
	}
	static := echo.WrapHandler(spaHandler(options.UI))
	e.GET("/*", static)
	e.HEAD("/*", static)

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

// stoppingGate は、状態を変える要求、Upgrade、SFTP data-plane要求を、
// 原子的に入場させるか断る。
//
// ルートの一覧は持たない。GET と HEAD 以外のすべて、実際の Upgrade、
// /api/v1/sftp/配下を対象にする。今日の WebSocket は /terminal/stream だが、
// 明日足されるものも誰も何も覚えていなくてもこの規則を継ぐ。
func (s *Server) stoppingGate(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		request := c.Request()
		// SFTP GET handlers are data-plane operations: they open SSH transports,
		// prepare plaintext spools and advance transfer jobs. Count every SFTP
		// request so shutdown cannot close the manager or release the engine while
		// a nominally read-only HTTP method still owns those resources.
		trackSFTP := strings.HasPrefix(request.URL.Path, "/api/v1/sftp/")
		if request.Method == http.MethodGet && !isUpgrade(request) && !trackSFTP {
			return next(c)
		}
		if request.Method == http.MethodHead && !trackSFTP {
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
// 呼び出し側の context では停止せず、アプリケーションが定めた停止順序に従う。
func (s *Server) Serve() error {
	return serveResult(s.http.Serve(s.listener))
}

func serveResult(err error) error {
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
