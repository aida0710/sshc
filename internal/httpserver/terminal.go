package httpserver

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/platform"
	"sshc/internal/terminal"
)

// StreamPath は、ブラウザが端末の生 I/O を運ぶ場所である。
//
// これは意図的に /api/ の下に置かれていない。**ブラウザは WebSocket の
// ハンドシェイクにカスタムヘッダを付けられない。** Security ミドルウェアは
// /api/ 配下の要求すべてに X-SSHC-CSRF を要求するので、そこに置いた
// アップグレードは必ず弾かれる。代わりに、直前の CSRF 付き要求が発行した
// 使い捨てのチケットで認可する。/cli/connect のエンドポイントが
// /api/ の外にあるのと同じ規則である。
const StreamPath = "/terminal/stream"

// TerminalHandlers は、埋め込みターミナルのセッションを提供する。
type TerminalHandlers struct {
	Registry *terminal.Registry
	Tickets  *terminal.Tickets
	// Connect は、alias ひとつ分の対話セッションを開く。
	//
	// **外部の ssh は起こさない。** プロセス内で SSH を話すので、確保する
	// PTY も無い。nil なら SSH のセッションは開けない。
	Connect Connector
	// Shell はローカルシェルの絶対パスを解決する。
	Shell func() (string, error)
	// Environment は、セッションが継ぐ環境である。これは利用者が自分で行った
	// であろう接続なので、検査が使う最小環境ではなく本人の環境を継ぐ。
	Environment func() []string
	// StartDirectory は、ローカルシェルが始まる場所を返す。
	//
	// **継がない。** エンジンの作業ディレクトリは、それを起こしたものが
	// たまたま居た場所である——デスクトップの外殻から起こせば `desktop/`、
	// launchd から起こせば `/` になる。**利用者はそのどれも選んでいない。**
	//
	// 関数なのは、設定が動いている最中に変わるからである。起動時に一度だけ
	// 読むと、変えた人は次に端末を開いても前の場所に立つ。nil ならこの
	// プロセスの作業ディレクトリを継ぐ。
	StartDirectory func() string
	// ExpectedOrigin は、アップグレードで完全一致を求める値である。
	ExpectedOrigin string
}

func registerTerminalRoutes(engine *echo.Echo, handlers TerminalHandlers) {
	engine.GET("/api/v1/terminal/sessions", handlers.List)
	engine.POST("/api/v1/terminal/sessions", handlers.Open)
	engine.POST("/api/v1/terminal/sessions/:id/stream", handlers.Ticket)
	engine.PATCH("/api/v1/terminal/sessions/:id", handlers.Rename)
	engine.DELETE("/api/v1/terminal/sessions/:id", handlers.Close)
	engine.GET(StreamPath, handlers.Stream)
}

// maxSessionIdentifier は、パスから受け取る識別子の長さを制限する。
const maxSessionIdentifier = 64

func describeSession(view terminal.View) api.TerminalSession {
	described := api.TerminalSession{
		Id:        view.ID,
		Kind:      api.TerminalSessionKind(view.Kind),
		Title:     view.Title,
		StartedAt: view.Started.UTC().Format(time.RFC3339),
	}
	if view.Alias != "" {
		alias := view.Alias
		described.Alias = &alias
	}
	if len(view.Forwards) > 0 {
		forwards := make([]api.TerminalForward, 0, len(view.Forwards))
		for _, forward := range view.Forwards {
			forwards = append(forwards, api.TerminalForward{
				Kind: forward.Kind, Listen: forward.Listen,
				To: forward.To, Problem: forward.Problem,
			})
		}
		described.Forwards = &forwards
	}
	if view.Exited != nil {
		described.Exited = &api.TerminalExit{
			Code:   view.Exited.Code,
			Signal: view.Exited.Signal,
			At:     view.Exited.At.UTC().Format(time.RFC3339),
		}
	}
	return described
}

func (h TerminalHandlers) list() api.TerminalSessionList {
	views := h.Registry.Sessions()
	sessions := make([]api.TerminalSession, 0, len(views))
	for _, view := range views {
		sessions = append(sessions, describeSession(view))
	}
	return api.TerminalSessionList{Sessions: sessions, MaxSessions: h.Registry.MaxSessions()}
}

// List は、生存と終了済みの両方を返す。
//
// 終了したセッションが残るのは、最後の出力を読めるようにするためである。ssh が
// 接続できなかった理由が読めるのは、そこだけになる。
func (h TerminalHandlers) List(c *echo.Context) error {
	return c.JSON(http.StatusOK, h.list())
}

// Open はセッションを開き、そのストリームのための使い捨てチケットを返す。
//
// action token は要求しない。vault ゲート（マスターパスワード）だけを条件と
// する。これは新しいゲートを作らなかったのではなく、以前の Terminal 起動に
// あったゲートを埋め込み版では外すという選択である。README にそう書いてある。
func (h TerminalHandlers) Open(c *echo.Context) error {
	var request api.OpenTerminalSessionRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	kind := terminal.Kind(request.Kind)
	if !terminal.ValidKind(kind) {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}

	size := terminal.Size{Cols: 80, Rows: 24}
	if request.Cols != nil && request.Rows != nil {
		candidate := terminal.Size{Cols: uint16(*request.Cols), Rows: uint16(*request.Rows)}
		if !candidate.Valid() {
			return problem(c, http.StatusBadRequest, "invalid_terminal_size")
		}
		size = candidate
	}

	spec, err := h.spec(kind, request.Alias, size)
	if err != nil {
		return h.startProblem(c, err)
	}
	session, err := h.Registry.Open(c.Request().Context(), spec)
	if err != nil {
		return h.startProblem(c, err)
	}

	ticket, err := h.Tickets.Issue(session.ID())
	if err != nil {
		// チケットを出せなければ誰も繋げない。開いたものは閉じる。
		_ = h.Registry.Close(session.ID())
		return problem(c, http.StatusInternalServerError, "terminal_start_failed")
	}
	return c.JSON(http.StatusCreated, api.OpenTerminalSessionResponse{
		Session: describeSession(session.View()), StreamTicket: ticket,
	})
}

// spec は、開こうとしているセッションひとつ分の起動一式を組み立てる。
func (h TerminalHandlers) spec(kind terminal.Kind, alias *string, size terminal.Size) (terminal.Spec, error) {
	if kind == terminal.KindShell {
		shell, err := h.resolveShell()
		if err != nil {
			return terminal.Spec{}, err
		}
		return terminal.Spec{
			Kind: terminal.KindShell, Title: shellTitle(shell), Size: size,
			Command: terminal.Command{
				Path: shell, Argv0: platform.LoginArgv0(shell), Env: h.environment(),
				Dir: h.startDirectory(),
			},
		}, nil
	}

	if alias == nil {
		return terminal.Spec{}, errMissingAlias
	}
	if err := platform.ValidateAlias(*alias); err != nil {
		return terminal.Spec{}, err
	}
	if h.Connect == nil {
		return terminal.Spec{}, terminal.ErrNoStarter
	}

	// 設定を読むのはここである。読めなければセッションを作らない——設定の
	// 問題は接続画面が表示できるので、端末に理由を書く必要が無い。接続そのものの
	// 出来事（届かない、認証が通らない）は、開いたセッションの中で語られる。
	target := *alias
	return terminal.Spec{
		Kind: terminal.KindSSH, Alias: target, Title: target, Size: size,
		Open: func(ctx context.Context, size terminal.Size) (terminal.Process, error) {
			// 確保が取り消されたなら、繋ぎに行かない。
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			// **成功したセッションの寿命は、要求から切り離す。** Dialer.Open は
			// すぐ返り、渡された context を非同期の接続のあいだ持ち続けるので、
			// 要求の context をそのまま渡すと、開いた HTTP ハンドラが返った瞬間に
			// SSH セッションが死ぬ。取り消す権利は Process.Close が持つ。
			sessionCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
			process, err := h.Connect(sessionCtx, target, size)
			if err != nil {
				cancel()
				return nil, err
			}
			return &sessionLifetime{Process: process, cancel: cancel}, nil
		},
	}, nil
}

// sessionLifetime は、セッションが生きているあいだだけ続く context を Process に
// 結び付ける。
type sessionLifetime struct {
	terminal.Process
	cancel context.CancelFunc
}

func (s *sessionLifetime) Close() error {
	err := s.Process.Close()
	s.cancel()
	return err
}

// ForceClose は、包んだ Process の強制停止を素通しする。
//
// **ここで落としてはならない。** 落とせば、レジストリからは強制停止を持たない
// Process に見え、締切に達しても輸送が切れなくなる。
func (s *sessionLifetime) ForceClose() error {
	var err error
	if forcer, ok := s.Process.(interface{ ForceClose() error }); ok {
		err = forcer.ForceClose()
	}
	s.cancel()
	return err
}

var errMissingAlias = errors.New("an ssh session needs an alias")

func shellTitle(shell string) string {
	for index := len(shell) - 1; index >= 0; index-- {
		if shell[index] == '/' {
			return shell[index+1:]
		}
	}
	return shell
}

func (h TerminalHandlers) resolveShell() (string, error) {
	if h.Shell == nil {
		return "", platform.ErrNoLoginShell
	}
	return h.Shell()
}

func (h TerminalHandlers) startDirectory() string {
	if h.StartDirectory == nil {
		return ""
	}
	return h.StartDirectory()
}

func (h TerminalHandlers) environment() []string {
	if h.Environment == nil {
		return nil
	}
	return h.Environment()
}

// startProblem は、開けなかった理由を、利用者が次に何をすればよいか分かる形に変える。
func (h TerminalHandlers) startProblem(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, terminal.ErrSessionLimit):
		// 黙って古いセッションを閉じることはしない。
		return problem(c, http.StatusConflict, "terminal_session_limit")
	case errors.Is(err, platform.ErrUnsafeAlias), errors.Is(err, errMissingAlias):
		return problem(c, http.StatusBadRequest, "unsafe_alias")
	}
	// 設定そのものが接続を許さない場合は、その理由を名指しする。
	// 「開けませんでした」だけでは、次に何をすればよいか分からない。
	if code, named := connectProblem(err); named {
		return problem(c, http.StatusUnprocessableEntity, code)
	}
	return problem(c, http.StatusInternalServerError, "terminal_start_failed")
}

// Ticket は、すでに開いているセッションへ繋ぎ直すためのチケットを出す。
//
// リロードしたページには、開いたときのチケットが残っていない——使い捨てで、
// しかも最初の接続で使い切られている。PTY はこの常駐プロセスの中で生きている
// ので、繋ぎ直す手段が要る。それがこれである。
//
// 終了済みのセッションにも出す。読めるものがあるからだ。
func (h TerminalHandlers) Ticket(c *echo.Context) error {
	id := c.Param("id")
	if id == "" || len(id) > maxSessionIdentifier {
		return problem(c, http.StatusNotFound, "terminal_session_not_found")
	}
	session, ok := h.Registry.Lookup(id)
	if !ok {
		return problem(c, http.StatusNotFound, "terminal_session_not_found")
	}
	ticket, err := h.Tickets.Issue(session.ID())
	if err != nil {
		return problem(c, http.StatusInternalServerError, "terminal_start_failed")
	}
	return c.JSON(http.StatusCreated, api.TerminalStreamTicket{StreamTicket: ticket})
}

// Close は、生存中なら子プロセスに SIGHUP、終了済みなら一覧から消す。
// Rename は、一覧に出す名前を変える。
//
// 変わるのは表示だけである。走っているプロセスにも、ssh の相手にも、この
// セッションの識別子にも触れない。名前が要るのは、同じ相手へ複数本開いたときに
// 行が見分けられなくなるからである。
//
// 名前は metadata へ書かない。セッションはこのプロセスの寿命までしか生きない
// ので、ディスクへ書けば必ず孤児が残る。
func (h TerminalHandlers) Rename(c *echo.Context) error {
	id := c.Param("id")
	if id == "" || len(id) > maxSessionIdentifier {
		return problem(c, http.StatusNotFound, "terminal_session_not_found")
	}
	var request api.RenameTerminalSessionRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	switch err := h.Registry.Rename(id, request.Title); {
	case errors.Is(err, terminal.ErrNotFound):
		return problem(c, http.StatusNotFound, "terminal_session_not_found")
	case errors.Is(err, terminal.ErrInvalidTitle):
		return problem(c, http.StatusBadRequest, "invalid_terminal_title")
	case err != nil:
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	return c.JSON(http.StatusOK, h.list())
}

func (h TerminalHandlers) Close(c *echo.Context) error {
	id := c.Param("id")
	if id == "" || len(id) > maxSessionIdentifier {
		return problem(c, http.StatusNotFound, "terminal_session_not_found")
	}
	if err := h.Registry.Close(id); err != nil {
		return problem(c, http.StatusNotFound, "terminal_session_not_found")
	}
	// 使われなかったチケットを、もう閉じたセッションに向けたまま残さない。
	h.Tickets.Forget(id)
	return c.JSON(http.StatusOK, h.list())
}
