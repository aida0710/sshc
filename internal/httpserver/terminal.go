package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/platform"
	"sshc/internal/snippets"
	"sshc/internal/terminal"
	"sshc/internal/validate"
)

// StreamPath は、ブラウザが端末の生 I/O を運ぶ場所である。
//
// これは意図的に /api/ の下に置かれていない。ブラウザは WebSocket の
// ハンドシェイクにカスタムヘッダを付けられない。Security ミドルウェアは
// /api/ 配下の要求すべてに X-SSHC-CSRF を要求するので、そこに置いた
// アップグレードは必ず弾かれる。代わりに、直前の CSRF 付き要求が発行した
// 使い捨てのチケットで認可する。/cli/connect のエンドポイントが
// /api/ の外にあるのと同じ規則である。
const StreamPath = "/terminal/stream"

// TerminalHandlers は、埋め込みターミナルのセッションを提供する。
type TerminalHandlers struct {
	Registry *terminal.Registry
	Tickets  *terminal.Tickets
	Snippets *snippets.Service
	Actions  ActionHandlers
	// Connect は、alias ひとつ分の対話セッションを開く。
	//
	// 外部の ssh は起動しない。プロセス内で SSH 接続を行うため、確保する
	// PTY も無い。nil なら SSH のセッションは開けない。
	Connect Connector
	// ConnectAgent starts an adapter-owned resume command on the same alias.
	ConnectAgent AgentConnector
	// ConnectionBinding detects alias retargeting before an opaque agent
	// reference can be tried on a different SSH destination.
	ConnectionBinding func(alias string) (string, error)
	// Shell はローカルシェルの絶対パスを解決する。
	Shell func() (string, error)
	// ShellProfiles returns only executables detected and validated on this machine.
	ShellProfiles func() []platform.ShellProfile
	// DefaultShellProfile returns the persisted stable ID. Empty means machine default.
	DefaultShellProfile func() string
	// Environment は、セッションが継ぐ環境である。これは利用者が自分で行った
	// であろう接続なので、検査が使う最小環境ではなくユーザー本人の環境を継ぐ。
	Environment func() []string
	// StartDirectory は、ローカルシェルが始まる場所を返す。
	//
	// 継がない。エンジンの作業ディレクトリは、それを起動したものが
	// たまたま居た場所である。シェルから起こせばその作業ディレクトリ、
	// launchd から起こせば `/` になる。利用者はそのどれも選んでいない。
	//
	// 関数なのは、設定が動いている最中に変わるからである。起動時に一度だけ
	// 読むと、変えたユーザーは次に端末を開いても前の場所に立つ。nil ならこの
	// プロセスの作業ディレクトリを継ぐ。
	StartDirectory func() string
	// ExpectedOrigin は、アップグレードで完全一致を求める値である。
	ExpectedOrigin string
	// Connected は、SSH接続とstream ticketの作成が成功したあとに呼ぶ。
	// 履歴の失敗で接続を失わせないため、エラーは呼び出し側が処理する。
	Connected func(alias string)
	// Startup returns an explicitly configured non-secret command. It is sent
	// only after authentication and remote shell startup have completed.
	Startup func(alias string) (string, bool)
}

func registerTerminalRoutes(engine *echo.Echo, handlers TerminalHandlers) {
	engine.GET("/api/v1/terminal/sessions", handlers.List)
	engine.GET("/api/v1/terminal/shell-profiles", handlers.ListShellProfiles)
	engine.POST("/api/v1/terminal/sessions", handlers.Open)
	if handlers.Snippets != nil && handlers.Actions.Sessions != nil {
		engine.POST("/api/v1/terminal/commands/preview", handlers.PreviewCommand)
		engine.POST("/api/v1/terminal/commands", handlers.DispatchCommand)
	}
	engine.POST("/api/v1/terminal/sessions/:id/stream", handlers.Ticket)
	engine.POST("/api/v1/terminal/sessions/:id/reconnect", handlers.Reconnect)
	engine.POST("/api/v1/terminal/sessions/:id/forwards", handlers.StartForward)
	engine.DELETE("/api/v1/terminal/sessions/:id/forwards/:forwardId", handlers.StopForward)
	engine.POST("/api/v1/terminal/sessions/:id/agent/resume", handlers.ResumeAgent)
	engine.GET("/api/v1/terminal/sessions/:id/control", handlers.Control)
	engine.PATCH("/api/v1/terminal/sessions/:id", handlers.Rename)
	engine.PUT("/api/v1/terminal/sessions/:id/title", handlers.SetTitle)
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
		State:     api.TerminalSessionState(view.State),
		Problem:   view.Problem,
		Presentation: &api.TerminalPresentation{
			DisplayTitle: view.Presentation.DisplayTitle,
			TitleSource:  api.TerminalPresentationTitleSource(view.Presentation.TitleSource),
			TitlePinned:  view.Presentation.TitlePinned,
		},
	}
	if view.Agent != nil {
		agent := &api.TerminalAgent{
			Kind: api.TerminalAgentKind(view.Agent.Kind), State: api.TerminalAgentState(view.Agent.State),
			Resumable: view.Agent.Resumable, ObservationVersion: int(view.Agent.ObservationVersion),
			SignalVersion: int(view.Agent.SignalVersion),
		}
		if view.Agent.CWD != "" {
			agent.Cwd = &view.Agent.CWD
		}
		if view.Agent.Model != "" {
			agent.Model = &view.Agent.Model
		}
		if view.Agent.SessionName != "" {
			agent.SessionName = &view.Agent.SessionName
		}
		if view.Agent.LastSignal != nil {
			agent.LastSignal = &api.TerminalAgentSignal{
				Kind:       api.TerminalAgentSignalKind(view.Agent.LastSignal.Kind),
				OccurredAt: view.Agent.LastSignal.OccurredAt.UTC(),
			}
		}
		described.Agent = agent
	}
	if view.Reconnect != nil {
		described.Reconnect = &api.TerminalReconnect{
			Attempt: view.Reconnect.Attempt,
			Limit:   view.Reconnect.Limit,
			RetryAt: view.Reconnect.RetryAt.UTC(),
			Problem: view.Reconnect.Problem,
		}
	}
	if view.Progress != nil {
		described.Progress = &api.TerminalConnectionProgress{
			Phase: api.TerminalConnectionProgressPhase(view.Progress.Phase),
			Alias: view.Progress.Alias, HostName: view.Progress.HostName,
			User: view.Progress.User, Hop: view.Progress.Hop, Hops: view.Progress.Hops,
		}
	}
	if view.Alias != "" {
		alias := view.Alias
		described.Alias = &alias
	}
	if len(view.Forwards) > 0 {
		forwards := make([]api.TerminalForward, 0, len(view.Forwards))
		for _, forward := range view.Forwards {
			forwards = append(forwards, api.TerminalForward{
				Id: forward.ID, Kind: forward.Kind, Listen: forward.Listen,
				To: forward.To, Problem: forward.Problem, Temporary: forward.Temporary,
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

// StartForward opens a loopback-only temporary forward on a connected SSH session.
func (h TerminalHandlers) StartForward(c *echo.Context) error {
	id := c.Param("id")
	if id == "" || len(id) > maxSessionIdentifier {
		return problem(c, http.StatusNotFound, "terminal_session_not_found")
	}
	var request api.StartTerminalForwardRequest
	if err := decodeJSON(c, &request); err != nil || (request.Kind != "local" && request.Kind != "dynamic") {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	destination := ""
	if request.Destination != nil {
		destination = *request.Destination
	}
	if request.ListenPort < 1 || request.ListenPort > 65535 || len(destination) > 512 ||
		(request.Kind == "local" && destination == "") ||
		(request.Kind == "dynamic" && destination != "") {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	_, err := h.Registry.StartForward(id, string(request.Kind), strconv.Itoa(request.ListenPort), destination)
	if err != nil {
		return terminalForwardProblem(c, err)
	}
	return c.JSON(http.StatusCreated, h.list())
}

// StopForward closes one listener without ending the owning terminal session.
func (h TerminalHandlers) StopForward(c *echo.Context) error {
	id, forwardID := c.Param("id"), c.Param("forwardId")
	if id == "" || len(id) > maxSessionIdentifier || forwardID == "" || len(forwardID) > maxSessionIdentifier {
		return problem(c, http.StatusNotFound, "terminal_forward_not_found")
	}
	if err := h.Registry.StopForward(id, forwardID); err != nil {
		return terminalForwardProblem(c, err)
	}
	return c.JSON(http.StatusOK, h.list())
}

func terminalForwardProblem(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, terminal.ErrNotFound):
		return problem(c, http.StatusNotFound, "terminal_session_not_found")
	case errors.Is(err, terminal.ErrForwardNotFound):
		return problem(c, http.StatusNotFound, "terminal_forward_not_found")
	case errors.Is(err, terminal.ErrInvalidForward):
		return problem(c, http.StatusUnprocessableEntity, "invalid_terminal_forward")
	case errors.Is(err, terminal.ErrNotConnected), errors.Is(err, terminal.ErrForwardUnavailable):
		return problem(c, http.StatusConflict, "terminal_forward_unavailable")
	default:
		return problemDetail(c, http.StatusConflict, "terminal_forward_bind_failed", err.Error())
	}
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

	spec, err := h.spec(kind, request.Alias, request.Cwd, size)
	if kind == terminal.KindShell && request.ProfileId != nil {
		spec, err = h.shellSpec(request.ProfileId, size)
	}
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
	if kind == terminal.KindSSH && h.Connected != nil {
		alias := *request.Alias
		session.WhenConnected(func() { h.Connected(alias) })
	}
	return c.JSON(http.StatusCreated, api.OpenTerminalSessionResponse{
		Session: describeSession(session.View()), StreamTicket: ticket,
	})
}

// SetTitle pins a user-selected display title, or unpins it when title is null.
// It changes only in-memory presentation state.
func (h TerminalHandlers) SetTitle(c *echo.Context) error {
	id := c.Param("id")
	if id == "" || len(id) > maxSessionIdentifier {
		return problem(c, http.StatusNotFound, "terminal_session_not_found")
	}
	var rawRequest struct {
		Title json.RawMessage `json:"title"`
	}
	if err := decodeJSON(c, &rawRequest); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if len(rawRequest.Title) == 0 {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	request := api.SetTerminalSessionTitleRequest{}
	if string(rawRequest.Title) != "null" {
		var title string
		if json.Unmarshal(rawRequest.Title, &title) != nil {
			return problem(c, http.StatusBadRequest, "invalid_request")
		}
		request.Title = &title
	}
	var err error
	if request.Title == nil {
		err = h.Registry.UnpinTitle(id)
	} else {
		err = h.Registry.Rename(id, *request.Title)
	}
	switch {
	case errors.Is(err, terminal.ErrNotFound):
		return problem(c, http.StatusNotFound, "terminal_session_not_found")
	case errors.Is(err, terminal.ErrInvalidTitle):
		return problem(c, http.StatusBadRequest, "invalid_terminal_title")
	case err != nil:
		return problem(c, http.StatusInternalServerError, "terminal_rename_failed")
	}
	return c.JSON(http.StatusOK, h.list())
}

// ResumeAgent replaces the process or opens a new pane using only the adapter's
// fixed argv. The browser supplies neither executable nor native reference.
func (h TerminalHandlers) ResumeAgent(c *echo.Context) error {
	id := c.Param("id")
	if id == "" || len(id) > maxSessionIdentifier {
		return problem(c, http.StatusNotFound, "terminal_session_not_found")
	}
	var request api.ResumeTerminalAgentRequest
	if err := decodeJSON(c, &request); err != nil || request.ObservationVersion < 1 {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	placement := terminal.AgentResumePlacement(request.Placement)
	if placement != terminal.AgentResumeSamePane && placement != terminal.AgentResumeNewPane {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	session, err := h.Registry.ResumeAgent(c.Request().Context(), id, uint64(request.ObservationVersion), placement)
	switch {
	case errors.Is(err, terminal.ErrNotFound):
		return problem(c, http.StatusNotFound, "terminal_session_not_found")
	case errors.Is(err, terminal.ErrAgentResumeStale):
		return problem(c, http.StatusConflict, "agent_resume_stale")
	case errors.Is(err, terminal.ErrAgentResumeSamePaneBusy):
		return problem(c, http.StatusConflict, "agent_resume_same_pane_busy")
	case errors.Is(err, terminal.ErrAgentResumeIdentityChanged):
		return problem(c, http.StatusConflict, "agent_resume_identity_changed")
	case errors.Is(err, terminal.ErrAgentResumeUnavailable), errors.Is(err, terminal.ErrReconnectUnavailable):
		return problem(c, http.StatusConflict, "agent_resume_unavailable")
	case errors.Is(err, terminal.ErrSessionLimit):
		return problem(c, http.StatusConflict, "terminal_session_limit")
	case errors.Is(err, terminal.ErrShuttingDown):
		return problem(c, http.StatusServiceUnavailable, "terminal_start_failed")
	case err != nil:
		if code, named := connectProblem(err); named {
			return problem(c, http.StatusUnprocessableEntity, code)
		}
		return problem(c, http.StatusInternalServerError, "terminal_start_failed")
	}
	ticket, err := h.Tickets.Issue(session.ID())
	if err != nil {
		if placement == terminal.AgentResumeNewPane {
			_ = h.Registry.Close(session.ID())
		}
		return problem(c, http.StatusInternalServerError, "terminal_start_failed")
	}
	return c.JSON(http.StatusCreated, api.OpenTerminalSessionResponse{
		Session: describeSession(session.View()), StreamTicket: ticket,
	})
}

// spec は、開こうとしているセッションひとつ分の起動一式を組み立てる。
func (h TerminalHandlers) spec(kind terminal.Kind, alias, cwd *string, size terminal.Size) (terminal.Spec, error) {
	if kind == terminal.KindShell {
		return h.shellSpec(nil, size)
	}

	if alias == nil {
		return terminal.Spec{}, errMissingAlias
	}
	if err := validate.Alias(*alias); err != nil {
		return terminal.Spec{}, err
	}
	if h.Connect == nil {
		return terminal.Spec{}, terminal.ErrNoStarter
	}
	initialDirectory := ""
	if cwd != nil {
		var err error
		initialDirectory, err = remoteWorkingDirectory(*cwd)
		if err != nil {
			return terminal.Spec{}, err
		}
	}

	// 設定を読むのはここである。読めなければセッションを作らない。設定の
	// 問題は接続画面が表示できるので、端末に理由を書く必要が無い。接続そのものの
	// 出来事（届かない、認証が通らない）は、開いたセッションの中で語られる。
	target := *alias
	spec := terminal.Spec{
		Kind: terminal.KindSSH, Alias: target, Title: target, Size: size,
		Open: func(ctx context.Context, size terminal.Size) (terminal.Process, error) {
			// 確保が取り消されたなら、繋ぎに行かない。
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			// 成功したセッションの寿命は、要求から切り離す。Dialer.Open は
			// すぐ返り、渡された context を非同期の接続のあいだ持ち続けるので、
			// 要求の context をそのまま渡すと、開いた HTTP ハンドラが返った瞬間に
			// SSH セッションが終了する。取り消す権利は Process.Close が持つ。
			sessionCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
			owned, lifetime, readier, asynchronous, err := ownTerminalProcess(sessionCtx, cancel, func(ctx context.Context) (terminal.Process, error) {
				return h.Connect(ctx, target, size)
			})
			if err != nil {
				return nil, err
			}
			// Open is also called by the registry when a disconnected transport is
			// re-established. Resolve and inject the startup snippet here so every
			// successful connection gets the same explicit startup automation.
			commands := make([]string, 0, 2)
			if initialDirectory != "" {
				commands = append(commands, "cd -- "+quotePOSIXShell(initialDirectory))
			}
			if h.Startup != nil {
				if command, ok := h.Startup(target); ok && command != "" {
					commands = append(commands, command)
				}
			}
			if len(commands) > 0 {
				go func() {
					if !asynchronous || receiveReady(readier.Ready()) == nil {
						for _, command := range commands {
							_, _ = lifetime.Write([]byte(command + "\r"))
						}
					}
				}()
			}
			return owned, nil
		},
		ReconnectError: func(err error) (bool, string) {
			if code, requiresAction := connectProblem(err); requiresAction {
				return false, code
			}
			return true, "reconnect_failed"
		},
	}
	if h.ConnectAgent != nil {
		binding := ""
		if h.ConnectionBinding != nil {
			resolved, err := h.ConnectionBinding(target)
			if err != nil {
				return terminal.Spec{}, err
			}
			binding = resolved
		}
		spec.Resume = func(ctx context.Context, size terminal.Size, kind terminal.AgentKind, reference string) (terminal.Process, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if h.ConnectionBinding != nil {
				current, err := h.ConnectionBinding(target)
				if err != nil {
					return nil, err
				}
				if current != binding {
					return nil, terminal.ErrAgentResumeIdentityChanged
				}
			}
			sessionCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
			owned, _, _, _, err := ownTerminalProcess(sessionCtx, cancel, func(ctx context.Context) (terminal.Process, error) {
				return h.ConnectAgent(ctx, target, kind, reference, size)
			})
			return owned, err
		}
	}
	if h.Startup != nil {
		command, ok := h.Startup(target)
		spec.ReplacementBusy = ok && command != ""
	}
	return spec, nil
}

func remoteWorkingDirectory(candidate string) (string, error) {
	if candidate == "" || len(candidate) > 4096 || !strings.HasPrefix(candidate, "/") || strings.ContainsAny(candidate, "\x00\r\n") {
		return "", errInvalidRemoteWorkingDirectory
	}
	cleaned := path.Clean(candidate)
	if cleaned != candidate {
		return "", errInvalidRemoteWorkingDirectory
	}
	return cleaned, nil
}

func quotePOSIXShell(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func (h TerminalHandlers) shellSpec(requested *string, size terminal.Size) (terminal.Spec, error) {
	profile, err := h.resolveShellProfile(requested)
	if err != nil {
		return terminal.Spec{}, err
	}
	return terminal.Spec{
		Kind: terminal.KindShell, Title: shellTitle(profile.Path), Size: size,
		Command: terminal.Command{
			Path: profile.Path, Argv0: profile.Argv0,
			Arguments: append([]string(nil), profile.Arguments...), Env: h.environment(),
			Dir: h.startDirectory(),
		},
	}, nil
}

func ownTerminalProcess(
	ctx context.Context,
	cancel context.CancelFunc,
	open func(context.Context) (terminal.Process, error),
) (terminal.Process, *sessionLifetime, terminal.Readier, bool, error) {
	process, err := open(ctx)
	if err != nil {
		cancel()
		return nil, nil, nil, false, err
	}
	lifetime := &sessionLifetime{Process: process, cancel: cancel}
	var owned terminal.Process = lifetime
	underlyingReady, asynchronous := process.(terminal.Readier)
	var readier terminal.Readier
	if asynchronous {
		readyLifetime := newReadySessionLifetime(lifetime, underlyingReady)
		owned, readier = readyLifetime, readyLifetime
	}
	return owned, lifetime, readier, asynchronous, nil
}

// sessionLifetime は、セッションが実行中あいだだけ続く context を Process に
// 結び付ける。
type sessionLifetime struct {
	terminal.Process
	cancel context.CancelFunc
}

type readySessionLifetime struct {
	*sessionLifetime
	done  chan struct{}
	mutex sync.Mutex
	err   error
}

// WriteExact preserves the lossless input capability of the in-process SSH
// session through the lifetime wrapper. Broadcast must fail closed when the
// underlying Process does not provide it.
func (s *sessionLifetime) WriteExact(ctx context.Context, input []byte) error {
	writer, ok := s.Process.(terminal.ExactInput)
	if !ok {
		return terminal.ErrExactInputUnavailable
	}
	return writer.WriteExact(ctx, input)
}

func newReadySessionLifetime(lifetime *sessionLifetime, underlying terminal.Readier) *readySessionLifetime {
	ready := &readySessionLifetime{sessionLifetime: lifetime, done: make(chan struct{})}
	go func() {
		err, _ := <-underlying.Ready()
		ready.mutex.Lock()
		ready.err = err
		ready.mutex.Unlock()
		close(ready.done)
	}()
	return ready
}

// Ready gives every observer its own one-result channel. The underlying SSH
// session emits one value, while both the registry and startup automation need
// to observe it without racing to consume that single value.
func (s *readySessionLifetime) Ready() <-chan error {
	result := make(chan error, 1)
	go func() {
		<-s.done
		s.mutex.Lock()
		err := s.err
		s.mutex.Unlock()
		result <- err
		close(result)
	}()
	return result
}

func receiveReady(ready <-chan error) error {
	err, _ := <-ready
	return err
}

// Forwards preserves the optional Process capability across the lifetime
// wrapper. Without this adapter an active SSH forward disappears from the
// terminal session API even though the underlying listener remains open.
func (s *sessionLifetime) Forwards() []terminal.Forward {
	if forwarder, ok := s.Process.(terminal.Forwarder); ok {
		return forwarder.Forwards()
	}
	return nil
}

// StartForward と StopForward も寿命wrapperを越えて同じ接続へ届ける。
// 一覧だけを透過して操作能力を落とすと、表示はできても管理APIは必ず失敗する。
func (s *sessionLifetime) StartForward(kind, listenPort, destination string) (terminal.Forward, error) {
	controller, ok := s.Process.(terminal.ForwardController)
	if !ok {
		return terminal.Forward{}, terminal.ErrForwardUnavailable
	}
	return controller.StartForward(kind, listenPort, destination)
}

func (s *sessionLifetime) StopForward(id string) error {
	controller, ok := s.Process.(terminal.ForwardController)
	if !ok {
		return terminal.ErrForwardUnavailable
	}
	return controller.StopForward(id)
}

// AwaitingPrompt preserves the optional pre-Ready input capability across the
// lifetime wrapper. Without it the registry would discard password and host-key answers.
func (s *sessionLifetime) AwaitingPrompt() bool {
	if prompting, ok := s.Process.(terminal.Prompting); ok {
		return prompting.AwaitingPrompt()
	}
	return false
}

func (s *sessionLifetime) Close() error {
	err := s.Process.Close()
	s.cancel()
	return err
}

// ForceClose は、包んだ Process の強制停止を素通しする。
//
// ここで落としてはならない。落とせば、レジストリからは強制停止を持たない
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
var errInvalidRemoteWorkingDirectory = errors.New("remote working directory is invalid")

// shellTitle は、タブに出す名前である。
//
// filepath でよい。ここに来るのは、このマシンで解決されたローカルシェルの
// 絶対パスだけなので、区切り文字はこの OS のものである。自前で `/` だけを
// 数えると、Windows のタブには `C:\Windows\System32\...\powershell.exe` が
// まるごと並ぶ。
func shellTitle(shell string) string { return filepath.Base(shell) }

func (h TerminalHandlers) resolveShell() (string, error) {
	if h.Shell == nil {
		return "", platform.ErrNoLoginShell
	}
	return h.Shell()
}

func (h TerminalHandlers) resolveShellProfile(requested *string) (platform.ShellProfile, error) {
	if h.ShellProfiles == nil {
		shell, err := h.resolveShell()
		if err != nil {
			return platform.ShellProfile{}, err
		}
		if requested != nil && *requested != "" && *requested != "default" {
			return platform.ShellProfile{}, platform.ErrUnknownShellProfile
		}
		return platform.ShellProfile{
			ID: "default", Label: shellTitle(shell), Path: shell,
			Argv0: platform.LoginArgv0(shell), Arguments: platform.LoginArguments(shell),
		}, nil
	}
	id := ""
	if requested != nil {
		id = *requested
	} else if h.DefaultShellProfile != nil {
		id = h.DefaultShellProfile()
	}
	profiles := h.ShellProfiles()
	profile, err := platform.ResolveShellProfile(profiles, id)
	if err != nil && requested == nil {
		// Metadata may be synced from another OS. An unavailable stored default
		// must not prevent the machine's verified login shell from opening.
		return platform.ResolveShellProfile(profiles, "default")
	}
	return profile, err
}

func (h TerminalHandlers) ListShellProfiles(c *echo.Context) error {
	profiles := []platform.ShellProfile(nil)
	if h.ShellProfiles != nil {
		profiles = h.ShellProfiles()
	}
	chosen := "default"
	if h.DefaultShellProfile != nil && h.DefaultShellProfile() != "" {
		chosen = h.DefaultShellProfile()
	}
	if _, err := platform.ResolveShellProfile(profiles, chosen); err != nil {
		chosen = "default"
	}
	answer := api.LocalShellProfileList{Profiles: make([]api.LocalShellProfile, 0, len(profiles))}
	for _, profile := range profiles {
		answer.Profiles = append(answer.Profiles, api.LocalShellProfile{
			Id: profile.ID, Label: profile.Label, Path: profile.Path,
			Arguments: append([]string(nil), profile.Arguments...), Default: profile.ID == chosen,
		})
	}
	return c.JSON(http.StatusOK, answer)
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
		// 暗黙に古いセッションを閉じることはしない。
		return problem(c, http.StatusConflict, "terminal_session_limit")
	case errors.Is(err, validate.ErrUnsafeAlias), errors.Is(err, errMissingAlias):
		return problem(c, http.StatusBadRequest, "unsafe_alias")
	case errors.Is(err, errInvalidRemoteWorkingDirectory):
		return problem(c, http.StatusBadRequest, "invalid_request")
	case errors.Is(err, platform.ErrUnknownShellProfile):
		return problem(c, http.StatusBadRequest, "local_shell_profile_unavailable")
	}
	// 設定により接続できない場合は、その理由を返す。
	// 「開けませんでした」だけでは、次に何をすればよいか分からない。
	if code, named := connectProblem(err); named {
		return problem(c, http.StatusUnprocessableEntity, code)
	}
	return problem(c, http.StatusInternalServerError, "terminal_start_failed")
}

// Ticket は、すでに開いているセッションへ繋ぎ直すためのチケットを出す。
//
// チケットは単回使用のため、ページを再読み込みしたクライアントへ新しい値を発行する。
// PTY は engine 内で継続しており、新しいチケットで再接続できる。
//
// 終了済みセッションにも発行し、残っている出力を再表示できるようにする。
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

// Reconnect は終了済みSSHセッションを、同じID、pane、scrollbackを保って
// 新しいshellとして開き直す。host keyと認証は保存済みのOpen経路で再検査する。
func (h TerminalHandlers) Reconnect(c *echo.Context) error {
	id := c.Param("id")
	if id == "" || len(id) > maxSessionIdentifier {
		return problem(c, http.StatusNotFound, "terminal_session_not_found")
	}
	_, err := h.Registry.Reconnect(c.Request().Context(), id)
	switch {
	case errors.Is(err, terminal.ErrNotFound):
		return problem(c, http.StatusNotFound, "terminal_session_not_found")
	case errors.Is(err, terminal.ErrReconnectUnavailable):
		return problem(c, http.StatusConflict, "terminal_reconnect_unavailable")
	case errors.Is(err, terminal.ErrSessionLimit):
		return problem(c, http.StatusConflict, "terminal_session_limit")
	case errors.Is(err, terminal.ErrShuttingDown):
		return problem(c, http.StatusServiceUnavailable, "terminal_start_failed")
	case err != nil:
		return h.startProblem(c, err)
	}
	return c.JSON(http.StatusOK, h.list())
}

// Close は、生存中なら子プロセスに SIGHUP、終了済みなら一覧から消す。
// Rename は、一覧に出す名前を変える。
//
// 変わるのは表示だけである。走っているプロセスにも、ssh の相手にも、この
// セッションの識別子にも触れない。名前が要るのは、同じ相手へ複数本開いたときに
// 行が見分けられなくなるからである。
//
// 名前は metadata へ保存しない。セッションは現在のプロセス内でだけ有効である。
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
		if errors.Is(err, terminal.ErrNotFound) {
			return problem(c, http.StatusNotFound, "terminal_session_not_found")
		}
		return problem(c, http.StatusInternalServerError, "terminal_close_failed")
	}
	// 使われなかったチケットを、もう閉じたセッションに向けたまま残さない。
	h.Tickets.Forget(id)
	return c.JSON(http.StatusOK, h.list())
}
