package httpserver

import (
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/platform"
	"sshc/internal/secret"
	"sshc/internal/terminal"
)

// StreamPath は、ブラウザが端末の生 I/O を運ぶ場所である。
//
// これは意図的に /api/ の下に置かれていない。**ブラウザは WebSocket の
// ハンドシェイクにカスタムヘッダを付けられない。** Security ミドルウェアは
// /api/ 配下の要求すべてに X-SSHC-CSRF を要求するので、そこに置いた
// アップグレードは必ず弾かれる。代わりに、直前の CSRF 付き要求が発行した
// 使い捨てのチケットで認可する。/cli/connect と askpass のエンドポイントが
// /api/ の外にあるのと同じ規則である。
const StreamPath = "/terminal/stream"

// TerminalHandlers は、埋め込みターミナルのセッションを提供する。
type TerminalHandlers struct {
	Registry *terminal.Registry
	Tickets  *terminal.Tickets
	// SSH は ssh の絶対パスを解決する。PATH は見ない。
	SSH func() (string, error)
	// Shell はローカルシェルの絶対パスを解決する。
	Shell func() (string, error)
	// Environment は、セッションが継ぐ環境である。これは利用者が自分で行った
	// であろう接続なので、検査が使う最小環境ではなく本人の環境を継ぐ。
	Environment func() []string
	// Passwords、KeyPassphraseTarget、AskpassHelper、AskpassURL は、保存済み
	// 鍵パスフレーズを持つ接続を武装させる。ConnectHandlers と同じ部品であり、
	// 欠けているものがあれば OpenSSH 自身が尋ねる普通の接続になる。
	Passwords           *secret.Service
	KeyPassphraseTarget func(alias string) (relativePath, promptPath, configSnapshot, evidence string, ok bool, err error)
	AskpassHelper       string
	AskpassURL          string
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
	session, err := h.Registry.Open(spec)
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
			},
		}, nil
	}

	if alias == nil {
		return terminal.Spec{}, errMissingAlias
	}
	if err := platform.ValidateAlias(*alias); err != nil {
		return terminal.Spec{}, err
	}
	ssh, err := h.resolveSSH()
	if err != nil {
		return terminal.Spec{}, err
	}

	// 保存済み鍵パスフレーズは、この 1 個の接続に対してヘルパーを武装させる。
	// トークンはここで発行され、その接続に使い切られる。
	var credential platform.AskpassCredential
	if h.AskpassHelper != "" && h.AskpassURL != "" {
		issued := issueAskpassCredential(h.Passwords, *alias, h.KeyPassphraseTarget)
		if issued.token != "" {
			credential = platform.AskpassCredential{
				Helper: h.AskpassHelper, URL: h.AskpassURL, Token: issued.token,
				Kind: issued.kind, IdentityFile: issued.identityFile, SSHConfig: issued.sshConfig,
			}
		}
	}

	// 組み立ては internal/platform が持つ。`sshc <alias>` が呼ぶのと同じ関数な
	// ので、環境から五つの変数を取り除く処理はこのリポジトリに一つしかない。
	built, cleanup, err := platform.InteractiveSSH(platform.InteractiveRequest{
		SSH: ssh, Alias: *alias, Inherited: h.environment(), Credential: credential,
	})
	if err != nil {
		return terminal.Spec{}, err
	}
	return terminal.Spec{
		Kind: terminal.KindSSH, Alias: *alias, Title: *alias, Size: size,
		Command: terminal.Command{Path: built.Path, Arguments: built.Arguments, Env: built.Env},
		Cleanup: cleanup,
	}, nil
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

func (h TerminalHandlers) resolveSSH() (string, error) {
	if h.SSH == nil {
		return "", platform.ErrInteractiveProgram
	}
	return h.SSH()
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
