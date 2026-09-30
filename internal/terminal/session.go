package terminal

import (
	"context"
	"sync"
	"time"
)

// readChunk は、PTY から一度に読むバイト数である。
const readChunk = 32 << 10

// Session は、開かれている端末ひとつである。
type Session struct {
	id            string
	kind          Kind
	alias         string
	title         string
	fallbackTitle string
	titleSource   TitleSource
	titlePinned   bool
	started       time.Time

	mutex         sync.Mutex
	inputMutex    sync.Mutex
	buffer        *Ring
	streams       map[*Stream]bool
	process       Process
	exited        *ExitInfo
	cleanup       func()
	state         State
	problem       string
	reconnectView *ReconnectView
	generation    uint64
	ready         *processReadiness
	// terminalTitle is the last OSC 0/1/2 title observed for the current
	// process generation. The browser applies the same sequence itself; the
	// engine records it so every client shows the same pane name.
	terminalTitle       string
	notificationVersion uint64
	lastNotification    *Notification

	connectedCallback func()
	connectedOnce     sync.Once

	reopen         func(ctx context.Context, size Size) (Process, error)
	reconnectError func(error) (retry bool, problem string)
	stopNotice     func(problem string) string
	startup        func() []string
	size           Size
	// reconnectAttempts は、今の切断から試した自動再接続の回数である。
	// ReconnectSettled のあいだ安定して繋がったら 0 に戻す。
	reconnectAttempts int
	reconnectCancel   context.CancelFunc
	now               func() time.Time
	// stopping は、繋ぎ直しを待っている最中に閉じられたことを伝える。
	stopping chan struct{}
	// discarded は、ユーザーが自分でこのコンソールを閉じたことを表す。
	discarded bool
	delay     func(attempt int) time.Duration
	// reconnectLimit は、自動再接続を何回まで試みてよいかを、試みるたびに返す。
	// 設定の変更を次の試みから効かせるため、値ではなく関数で持つ。
	reconnectLimit func() int

	// done は pump が終わったことを示す。テストと停止処理だけが待つ。
	done chan struct{}
}

type processReadiness struct {
	done        chan struct{}
	err         error
	connectedAt time.Time
	// userInitiated は、この世代を利用者の操作（開く、手動の再接続）が始めたことを
	// 表す。自動再接続の試みではないので、Ready の失敗を再接続の予算に数えない。
	userInitiated bool
}

// View は、一覧に出すためのセッションひとつ分である。
type View struct {
	ID        string
	Kind      Kind
	Alias     string
	Title     string
	Started   time.Time
	Exited    *ExitInfo
	State     State
	Problem   string
	Reconnect *ReconnectView
	// Progress is present while an SSH process is connecting or reconnecting.
	Progress *ConnectionProgress
	// Forwards は、このセッションが開いている転送である。
	Forwards     []Forward
	Presentation Presentation
	// NotificationVersion increases for every notification a program asked
	// for; LastNotification is the most recent one.
	NotificationVersion uint64
	LastNotification    *Notification
}

// CommandTarget is the server-derived identity of one live terminal at
// confirmation time. Generation changes whenever a reconnect installs a new
// Process, even though the public session ID remains the same.
type CommandTarget struct {
	ID         string
	Kind       Kind
	Alias      string
	Title      string
	Generation uint64
}

func (s *Session) ID() string { return s.id }

// WhenConnected registers work which may run once, after an asynchronous SSH
// Process has actually authenticated and started its remote shell. The caller
// may register after Ready has fired; this is needed because a stream ticket
// must be issued before a successful connection may be recorded.
func (s *Session) WhenConnected(callback func()) {
	if callback == nil {
		return
	}
	s.mutex.Lock()
	s.connectedCallback = callback
	connected := s.state == StateConnected
	s.mutex.Unlock()
	if connected {
		s.signalConnected()
	}
}

func (s *Session) signalConnected() {
	s.mutex.Lock()
	callback := s.connectedCallback
	connected := s.state == StateConnected
	s.mutex.Unlock()
	if connected && callback != nil {
		s.connectedOnce.Do(callback)
	}
}

// Exit は終了理由を返す。実行中の場合は nil を返す。
func (s *Session) Exit() *ExitInfo {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.exited == nil {
		return nil
	}
	info := *s.exited
	return &info
}

// Done は現在のprocess世代のpumpが終了したとき閉じる。返したchannelは、手動再接続が
// 新しい世代を開始しても元の世代を指し続けるため、呼び出し側は待ち始めた終了だけを
// 決定的に観測できる。
func (s *Session) Done() <-chan struct{} {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.done
}

func (s *Session) Live() bool { return s.Exit() == nil }

// View は一覧に出すための写しである。
func (s *Session) View() View {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	view := View{
		ID: s.id, Kind: s.kind, Alias: s.alias, Title: s.title,
		Started: s.started, State: s.state, Problem: s.problem,
		Presentation:        Presentation{DisplayTitle: s.title, TitleSource: s.titleSource, TitlePinned: s.titlePinned},
		NotificationVersion: s.notificationVersion,
	}
	if s.lastNotification != nil {
		notification := *s.lastNotification
		view.LastNotification = &notification
	}
	if s.reconnectView != nil {
		status := *s.reconnectView
		view.Reconnect = &status
	}
	if s.exited != nil {
		info := *s.exited
		view.Exited = &info
	}
	if forwarder, ok := s.process.(Forwarder); ok {
		view.Forwards = forwarder.Forwards()
	}
	if progressing, ok := s.process.(Progressing); ok &&
		(s.state == StateConnecting || s.state == StateReconnecting) {
		progress := progressing.ConnectionProgress()
		if progress.Phase != "" {
			view.Progress = &progress
		}
	}
	return view
}

// StartForward は現在接続済みのprocess世代へ一時転送を追加する。
// process交換と同時に古い輸送へlistenerを残さないよう、世代を所有するlock内で行う。
func (s *Session) StartForward(kind, listenPort, destination string) (Forward, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.exited != nil || s.process == nil || s.state != StateConnected {
		return Forward{}, ErrNotConnected
	}
	controller, ok := s.process.(ForwardController)
	if !ok {
		return Forward{}, ErrForwardUnavailable
	}
	return controller.StartForward(kind, listenPort, destination)
}

// StopForward は現在process世代の転送ひとつだけを閉じる。
func (s *Session) StopForward(id string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.exited != nil || s.process == nil || s.state != StateConnected {
		return ErrNotConnected
	}
	controller, ok := s.process.(ForwardController)
	if !ok {
		return ErrForwardUnavailable
	}
	return controller.StopForward(id)
}

// Write は打鍵を PTY へ渡す。
func (s *Session) Write(p []byte) (int, error) {
	s.inputMutex.Lock()
	defer s.inputMutex.Unlock()
	s.mutex.Lock()
	process, exited, state := s.process, s.exited, s.state
	s.mutex.Unlock()
	if exited != nil || process == nil {
		if exited == nil && (state == StateConnecting || state == StateReconnecting) {
			return len(p), nil
		}
		return 0, ErrExited
	}
	if state == StateConnecting || state == StateReconnecting {
		prompting, ok := process.(Prompting)
		if !ok || !prompting.AwaitingPrompt() {
			// Ready前の通常入力を溜めない。明示的な認証promptだけが例外である。
			return len(p), nil
		}
	}
	return process.Write(p)
}

// CommandTarget returns a binding only for a connected Process capable of exact
// input. Broadcast preview uses this instead of a destination name so it can
// never open a replacement terminal implicitly.
func (s *Session) CommandTarget() (CommandTarget, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.exited != nil || s.process == nil || s.state != StateConnected {
		return CommandTarget{}, ErrNotConnected
	}
	if _, ok := s.process.(ExactInput); !ok {
		return CommandTarget{}, ErrExactInputUnavailable
	}
	alias := s.alias
	if alias == "" && s.kind == KindShell {
		alias = "localhost"
	}
	return CommandTarget{ID: s.id, Kind: s.kind, Alias: alias, Title: s.title, Generation: s.generation}, nil
}

// WriteCommandInput writes to the exact Process generation captured by a
// preview. submit=false inserts bytes without a carriage return.
func (s *Session) WriteCommandInput(ctx context.Context, generation uint64, command string, submit bool) error {
	if len(command) == 0 || len(command) > MaxCommandBytes {
		return ErrCommandTooLarge
	}
	extra := 0
	if submit {
		extra = 1
	}
	payload := make([]byte, len(command)+extra)
	copy(payload, command)
	if submit {
		payload[len(command)] = '\r'
	}

	s.inputMutex.Lock()
	defer s.inputMutex.Unlock()
	s.mutex.Lock()
	if s.generation != generation {
		s.mutex.Unlock()
		return ErrGenerationChanged
	}
	process, exited, state := s.process, s.exited, s.state
	s.mutex.Unlock()
	if exited != nil || process == nil || state != StateConnected {
		return ErrNotConnected
	}
	writer, ok := process.(ExactInput)
	if !ok {
		return ErrExactInputUnavailable
	}
	return writer.WriteExact(ctx, payload)
}

// Resize は PTY の大きさを変え、その大きさを覚える。再接続や置き換えで開く
// 新しい PTY はこの最新の大きさで始まる。再接続を待っている間（PTY がまだ
// 無い間）の変更も覚える。ブラウザは一度送った大きさを送り直さないので、
// ここで捨てると再接続後の PTY が古い大きさになり、リモートのプログラムが
// 見えない行へ描く。
func (s *Session) Resize(size Size) error {
	if !size.Valid() {
		return ErrInvalidSize
	}
	s.mutex.Lock()
	process, exited := s.process, s.exited
	if exited == nil {
		s.size = size
	}
	s.mutex.Unlock()
	if exited != nil {
		return ErrExited
	}
	if process == nil {
		// 再接続待ち。次の PTY は覚えた大きさで開く。
		return nil
	}
	return process.Resize(size)
}

// Discard は、このセッションがユーザーの意思で閉じられたことを記録する。
func (s *Session) Discard() {
	s.mutex.Lock()
	s.discarded = true
	s.mutex.Unlock()
}

// Discarded は、ユーザーが自分で閉じたかを返す。
func (s *Session) Discarded() bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.discarded
}

// Hangup は子プロセスへ SIGHUP を送る。終了そのものは pump が観測する。
func (s *Session) Hangup() error {
	s.stopReconnecting()
	s.mutex.Lock()
	process, exited := s.process, s.exited
	s.mutex.Unlock()
	if exited != nil || process == nil {
		return nil
	}
	return process.Hangup()
}

// stoppedLocked は、このセッションの process をもう公開しないと決まったかを返す。
// 利用者が閉じたか（discarded）、再接続を止めたか engine が止まるか（stopping）で決まる。
// 呼び出し側が mutex を持つ。
func (s *Session) stoppedLocked() bool {
	if s.discarded {
		return true
	}
	select {
	case <-s.stopping:
		return true
	default:
		return false
	}
}

// forceClose は、自動再接続を止め、生きている Process を強制停止する。
func (s *Session) forceClose() error {
	s.stopReconnecting()
	s.mutex.Lock()
	process, exited := s.process, s.exited
	s.mutex.Unlock()
	if exited != nil || process == nil {
		return nil
	}
	return process.ForceClose()
}

// observeProcess installs the readiness observation for the current Process.
// pending is the state exposed until Ready succeeds. Processes without a Ready
// capability retain the historical synchronous-Open contract.
func (s *Session) observeProcess(pending State, successMessage string) {
	s.mutex.Lock()
	// A replacement shell announces its own title; the previous one must not
	// linger on the pane while it starts.
	s.resetTerminalTitleLocked()
	s.generation++
	generation := s.generation
	process := s.process
	readier, asynchronous := process.(Readier)
	if !asynchronous {
		s.ready = nil
		s.state = StateConnected
		s.problem = ""
		s.reconnectView = nil
		s.mutex.Unlock()
		if successMessage != "" {
			s.publish([]byte(successMessage))
		}
		s.signalConnected()
		s.sendStartup(process)
		return
	}
	observed := &processReadiness{done: make(chan struct{}), userInitiated: pending == StateConnecting}
	s.ready = observed
	s.state = pending
	s.mutex.Unlock()

	go func() {
		<-readier.Ready()
		err := readier.ReadyErr()
		observed.err = err
		if err == nil {
			observed.connectedAt = s.now()
		}
		s.mutex.Lock()
		stopped := s.stoppedLocked() || s.exited != nil
		current := s.generation == generation && s.ready == observed && !stopped
		if current && err == nil {
			s.state = StateConnected
			s.problem = ""
			s.reconnectView = nil
		}
		s.mutex.Unlock()
		close(observed.done)
		if current && err == nil {
			if successMessage != "" {
				s.publish([]byte(successMessage))
			}
			s.signalConnected()
			s.sendStartup(process)
		}
	}()
}

// sendStartup は、Spec.Startup のコマンドを、使える状態になった Process へ送る。
//
// 世代ごとに呼ぶので、再接続した新しいシェルにも同じコマンドが届く。SSH の
// Process では Ready を待ってから送るので、認証の問いへ答えとして流れない。
func (s *Session) sendStartup(process Process) {
	if s.startup == nil {
		return
	}
	for _, command := range s.startup() {
		_, _ = process.Write([]byte(command + "\r"))
	}
}

// pump は PTY を読み、バッファへ書き、アタッチしているものへ配る。
func (s *Session) pump(now func() time.Time) {
	defer close(s.done)
	connectedAt := s.started
	for {
		s.mutex.Lock()
		process, ready, generation := s.process, s.ready, s.generation
		s.mutex.Unlock()
		observer := newOSCObserver(
			func(title string) { s.acceptTitle(generation, title) },
			func(title, body string) { s.acceptNotification(generation, title, body, now()) },
		)
		buffer := make([]byte, readChunk)
		for {
			read, err := process.Read(buffer)
			if read > 0 {
				observer.Observe(buffer[:read])
				s.publish(buffer[:read])
			}
			if err != nil {
				break
			}
		}
		// このプロセスはもう出力しない。次のシェルの出力より前にモードを戻す。
		s.publish([]byte(leftoverModeReset))
		info := process.Wait()
		// 終わり方の文は、戻したあとの通常の画面へ書く。
		if info.Notice != "" {
			s.publish([]byte(info.Notice))
		}
		var connectionErr error
		if ready != nil {
			<-ready.done
			connectionErr = ready.err
		}
		if info.At.IsZero() {
			info.At = now()
		}
		_ = process.Close()
		settledAt := connectedAt
		if ready != nil && !ready.connectedAt.IsZero() {
			settledAt = ready.connectedAt
		}
		// 握手に長く掛かった時間は安定稼働に数えない。Ready に失敗した
		// 再接続で予算を戻すと、失敗し続ける接続が永久に回り続ける。
		if connectionErr == nil && info.At.Sub(settledAt) >= ReconnectSettled {
			s.mutex.Lock()
			s.reconnectAttempts = 0
			s.mutex.Unlock()
		}

		if connectionErr != nil && ready.userInitiated {
			// 一度も繋がっていない接続を「切れました」として試し直さない。
			// 失敗の理由は接続ログとしてターミナルに出ている。
			s.recordConnectionFailure(connectionErr)
			s.finish(info)
			return
		}
		if !s.reconnect(info, connectionErr, now) {
			s.finish(info)
			return
		}
		connectedAt = now()
	}
}

// problemConnectFailed は、利用者が始めた接続が、固定の problem code を持たない
// 理由（名前解決、接続の拒否、タイムアウトなど）で失敗したことを表す。
const problemConnectFailed = "connect_failed"

// recordConnectionFailure は、利用者が始めた接続の失敗を problem に残す。
// 自動再接続はしていないので、再接続を止めたという文は書かない。
func (s *Session) recordConnectionFailure(err error) {
	problem := s.connectionFailureProblem(err)
	s.mutex.Lock()
	s.problem = problem
	s.mutex.Unlock()
}

// connectionFailureProblem は、利用者が始めた接続（最初の接続と手動の再接続）の
// 失敗を problem code にする。自動で試し直さないので、試し直せる失敗も
// reconnect_failed ではなく connect_failed で表す。Ready の前に開けなかったときも、
// Ready で失敗したときも同じ code にする。
func (s *Session) connectionFailureProblem(err error) string {
	if retry, code := s.classifyReconnectFailure(err); !retry {
		return code
	}
	return problemConnectFailed
}

// abandonProcess は、公開しないと決めた process を強制停止して閉じる。
// 終了の観測は呼び出し側が行う。
func abandonProcess(process Process) {
	_ = process.ForceClose()
	_ = process.Close()
}

// abandonAndWait は、pump が観測しない process を捨て、終わるまで待つ。
func abandonAndWait(process Process) {
	abandonProcess(process)
	process.Wait()
}

func (s *Session) finish(info ExitInfo) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.exited != nil {
		return
	}
	s.exited = &info
	s.state = StateExited
	s.reconnectView = nil
	s.closeStreamsLocked()
	if s.cleanup != nil {
		cleanup := s.cleanup
		s.cleanup = nil
		cleanup()
	}
}
