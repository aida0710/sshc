package terminal

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// streamDepth は、ひとつのアタッチが溜め込めるチャンクの数である。
const streamDepth = 256

// readChunk は、PTY から一度に読むバイト数である。
const readChunk = 32 << 10

// Stream は、ひとつのアタッチである。
type Stream struct {
	output  chan []byte
	closed  sync.Once
	dropped atomic.Bool
}

func (s *Stream) Output() <-chan []byte { return s.output }

// Dropped は、このアタッチが追いつけずに落とされたかを報告する。
func (s *Stream) Dropped() bool { return s.dropped.Load() }

func (s *Stream) close() { s.closed.Do(func() { close(s.output) }) }

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

	reopen          func(ctx context.Context, size Size) (Process, error)
	reconnectError  func(error) (retry bool, problem string)
	size            Size
	retries         int
	reconnectCancel context.CancelFunc
	now             func() time.Time
	// stopping は、繋ぎ直しを待っている最中に閉じられたことを伝える。
	stopping chan struct{}
	// discarded は、ユーザーが自分でこのコンソールを閉じたことを表す。
	discarded bool
	delay     func(attempt int) time.Duration
	// attempts は、繋ぎ直しを何回まで試みてよいかを、試みるたびに返す。
	attempts func() int

	// done は pump が終わったことを示す。テストと停止処理だけが待つ。
	done chan struct{}
}

type processReadiness struct {
	done        chan struct{}
	err         error
	connectedAt time.Time
}

// TitleSource says where the display title came from so the UI can offer
// "return to the automatic name" only when a user pinned one.
type TitleSource string

const (
	TitleUser       TitleSource = "user"
	TitleTerminal   TitleSource = "terminal"
	TitleConnection TitleSource = "connection"
	TitleFallback   TitleSource = "fallback"
)

// Presentation is the display-only view of the title state.
type Presentation struct {
	DisplayTitle string
	TitleSource  TitleSource
	TitlePinned  bool
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

// Rename は一覧に出す名前を変える。
func (s *Session) Rename(title string) error {
	cleaned, err := CleanTitle(title)
	if err != nil {
		return err
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.title = cleaned
	s.titleSource = TitleUser
	s.titlePinned = true
	return nil
}

// UnpinTitle returns the display title to the title the terminal set or the
// connection fallback without touching the running process.
func (s *Session) UnpinTitle() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.titlePinned = false
	s.recomputeTitleLocked()
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

// CanAttachFrom reports whether cursor belongs to the output range written by
// this session. An old cursor is valid and will be marked truncated; a cursor
// ahead of the writer is not.
func (s *Session) CanAttachFrom(cursor uint64) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.buffer.CanReadFrom(cursor)
}

// AttachFrom atomically returns only output after cursor and then follows live
// output. Registering the stream under the same lock as the range read leaves
// no gap between replay and live delivery.
func (s *Session) AttachFrom(cursor uint64) (RingRead, *Stream, bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	replay, ok := s.buffer.ReadAvailableFrom(cursor)
	if !ok {
		return RingRead{}, nil, false
	}
	stream := &Stream{output: make(chan []byte, streamDepth)}
	if s.exited != nil {
		// 終了済みのセッションにライブの出力は無い。読めるものを渡してから閉じる。
		stream.close()
		return replay, stream, true
	}
	s.streams[stream] = true
	return replay, stream, true
}

// Detach は接続を解除する。セッション自体は継続する。
func (s *Session) Detach(stream *Stream) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.streams[stream] {
		delete(s.streams, stream)
	}
	stream.close()
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

// Resize は TIOCSWINSZ を発行する。
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

// closeStreams は、アタッチしているものをすべて外す。
func (s *Session) closeStreams() {
	s.mutex.Lock()
	streams := make([]*Stream, 0, len(s.streams))
	for stream := range s.streams {
		streams = append(streams, stream)
	}
	s.streams = map[*Stream]bool{}
	s.mutex.Unlock()
	for _, stream := range streams {
		stream.close()
	}
}

// forceClose は、Process が持っていればその強制停止を呼ぶ。
func (s *Session) forceClose() error {
	s.stopReconnecting()
	s.mutex.Lock()
	process, exited := s.process, s.exited
	s.mutex.Unlock()
	if exited != nil || process == nil {
		return nil
	}
	forcer, ok := process.(forceCloser)
	if !ok {
		// 外部実装との互換用fallbackである。sshcが生成するProcessはすべて
		// ForceCloseを持つため、実運用ではこの経路へ入らない。
		return process.Hangup()
	}
	return forcer.ForceClose()
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
	readier, asynchronous := s.process.(Readier)
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
		return
	}
	observed := &processReadiness{done: make(chan struct{})}
	s.ready = observed
	s.state = pending
	s.mutex.Unlock()

	go func() {
		err, open := <-readier.Ready()
		if !open {
			err = nil
		}
		observed.err = err
		if err == nil {
			observed.connectedAt = s.now()
		}
		s.mutex.Lock()
		stopped := s.discarded || s.exited != nil
		select {
		case <-s.stopping:
			stopped = true
		default:
		}
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
		}
	}()
}

// pump は PTY を読み、バッファへ書き、アタッチしているものへ配る。
const MaxReconnects = 5

// ReconnectSettled は、再接続予算を戻してよい連続稼働時間である。短時間に
// 切断を繰り返す接続は有限回で止め、安定していた接続の過去の失敗は持ち越さない。
const ReconnectSettled = 10 * time.Second

// ReconnectJitterMaxPercent は、再接続の待ち時間に掛かる揺らぎの上限（%）。
const ReconnectJitterMaxPercent = 120

// ReconnectBackoff は、試みのあいだに置く間隔である。
// ReconnectBackoff は、n 回目の再接続までに待つ基準の秒数。表の末尾以降は最後の
// 値を繰り返す。設定画面の文言はこの表から総所要時間を言う。
var ReconnectBackoff = []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second, 15 * time.Second}

// NormaliseReconnects は、範囲の外にある回数を天井へ戻す。
func NormaliseReconnects(attempts int) int {
	if attempts < 0 || attempts > MaxReconnects {
		return MaxReconnects
	}
	return attempts
}

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
		info := process.Wait()
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
			s.retries = 0
			s.mutex.Unlock()
		}

		if !s.reconnect(info, connectionErr, now) {
			s.finish(info)
			return
		}
		connectedAt = now()
	}
}

// acceptTitle records an OSC 0/1/2 title. An empty title means the program
// cleared it, so the pane falls back to its connection name.
func (s *Session) acceptTitle(generation uint64, title string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if generation != s.generation || s.exited != nil {
		return
	}
	s.terminalTitle = title
	s.recomputeTitleLocked()
}

// acceptNotification records an OSC 9/99/777 notification. Clients compare
// NotificationVersion between polls, so every request counts even when the
// text repeats.
func (s *Session) acceptNotification(generation uint64, title, body string, occurredAt time.Time) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if generation != s.generation || s.exited != nil {
		return
	}
	s.notificationVersion++
	s.lastNotification = &Notification{Title: title, Body: body, OccurredAt: occurredAt}
}

func (s *Session) resetTerminalTitleLocked() {
	if s.terminalTitle == "" {
		return
	}
	s.terminalTitle = ""
	s.recomputeTitleLocked()
}

func (s *Session) recomputeTitleLocked() {
	if s.titlePinned {
		s.titleSource = TitleUser
		return
	}
	if s.terminalTitle != "" {
		s.title = s.terminalTitle
		s.titleSource = TitleTerminal
		return
	}
	s.title = s.fallbackTitle
	if s.alias != "" {
		s.titleSource = TitleConnection
	} else {
		s.titleSource = TitleFallback
	}
}

// reconnect は、落ちた輸送を繋ぎ直せたなら真を返す。
// StopReconnecting abandons the automatic reconnect loop while it is waiting
// or dialing. The pane stays open in the exited state so the user can decide
// later whether to reconnect by hand or close it.
func (s *Session) StopReconnecting() error {
	s.mutex.Lock()
	if s.state != StateReconnecting || s.exited != nil {
		s.mutex.Unlock()
		return ErrNotReconnecting
	}
	s.problem = "reconnect_stopped"
	handshaking := s.handshakingProcessLocked()
	s.mutex.Unlock()
	s.stopReconnecting()
	if handshaking != nil {
		// reopen は握手前に Process を返す。Ready を待つ側は stopping を見て
		// connected にしないだけで、process 自体は生きて入力を捨て続ける。
		// 閉じて pump に exited まで進ませ、手動の再接続を使える状態にする。
		abandonProcess(handshaking)
	}
	s.publish([]byte("\r\n[sshc] 再接続を停止しました。\r\n"))
	return nil
}

// handshakingProcessLocked は、reopen が返した後で Ready がまだ決まっていない
// process を返す。待機中や dial 中、または確定後は nil を返す。
func (s *Session) handshakingProcessLocked() Process {
	if s.process == nil || s.ready == nil {
		return nil
	}
	select {
	case <-s.ready.done:
		return nil
	default:
		return s.process
	}
}

// abandonProcess は、公開しないと決めた process を強制停止して閉じる。
// 終了の観測は呼び出し側が行う。
func abandonProcess(process Process) {
	if forcer, ok := process.(forceCloser); ok {
		_ = forcer.ForceClose()
	}
	_ = process.Close()
}

func (s *Session) stopReconnecting() {
	s.mutex.Lock()
	cancel := s.reconnectCancel
	select {
	case <-s.stopping:
	default:
		close(s.stopping)
	}
	s.mutex.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *Session) reconnect(info ExitInfo, connectionErr error, now func() time.Time) bool {
	// Dialer.Open は握手前に Process を返す。したがって終了コードが通常の
	// transport loss でなくても、Ready の失敗は接続失敗として扱う。
	if !info.Lost() && connectionErr == nil {
		return false
	}
	if connectionErr != nil {
		retry, problem := true, "reconnect_failed"
		if s.reconnectError != nil {
			retry, problem = s.reconnectError(connectionErr)
		}
		s.mutex.Lock()
		if s.reconnectView != nil {
			s.reconnectView.Problem = problem
		}
		if !retry {
			s.problem = problem
		}
		s.mutex.Unlock()
		if !retry {
			return false
		}
	}
	for {
		s.mutex.Lock()
		reopen, attempt := s.reopen, s.retries
		stopping := s.exited != nil
		s.mutex.Unlock()
		select {
		case <-s.stopping:
			stopping = true
		default:
		}

		limit := MaxReconnects
		if s.attempts != nil {
			limit = NormaliseReconnects(s.attempts())
		}
		if reopen == nil || stopping || attempt >= limit {
			if reopen != nil && limit > 0 && attempt >= limit {
				s.mutex.Lock()
				s.problem = "reconnect_exhausted"
				s.mutex.Unlock()
				s.publish([]byte("\r\n[sshc] 再接続できる回数の上限に達しました。\r\n"))
			}
			return false
		}

		wait := ReconnectBackoff[min(attempt, len(ReconnectBackoff)-1)]
		if s.delay != nil {
			wait = s.delay(attempt)
		}
		retryAt := now().Add(wait)
		s.mutex.Lock()
		s.process = nil
		s.state = StateReconnecting
		s.reconnectView = &ReconnectView{Attempt: attempt + 1, Limit: limit, RetryAt: retryAt}
		s.mutex.Unlock()
		seconds := int((wait + time.Second - 1) / time.Second)
		s.publish([]byte(fmt.Sprintf(
			"\r\n[sshc] SSH 接続が切れました。%d 秒後に再接続します（%d/%d）。\r\n",
			seconds, attempt+1, limit)))

		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-s.stopping:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return false
		}

		attemptCtx, cancel := context.WithCancel(context.Background())
		s.mutex.Lock()
		select {
		case <-s.stopping:
			s.mutex.Unlock()
			cancel()
			return false
		default:
		}
		s.reconnectCancel = cancel
		// The browser may have been resized during the wait; the new shell
		// must start at that size, so it is read only after waiting.
		size := s.size
		s.mutex.Unlock()
		process, err := reopen(attemptCtx, size)
		s.mutex.Lock()
		s.reconnectCancel = nil
		stopped := s.discarded
		select {
		case <-s.stopping:
			stopped = true
		default:
		}
		s.mutex.Unlock()
		cancel()
		if process != nil && stopped {
			abandonProcess(process)
			process.Wait()
			return false
		}
		if err != nil {
			retry, problem := true, "reconnect_failed"
			if s.reconnectError != nil {
				retry, problem = s.reconnectError(err)
			}
			s.mutex.Lock()
			s.retries++
			if s.reconnectView != nil {
				s.reconnectView.Problem = problem
			}
			if !retry {
				s.problem = problem
			}
			s.mutex.Unlock()
			s.publish([]byte("\r\n[sshc] " + err.Error() + "\r\n"))
			if !retry {
				return false
			}
			continue
		}

		s.mutex.Lock()
		stopped = s.discarded
		select {
		case <-s.stopping:
			stopped = true
		default:
		}
		if !stopped {
			s.process = process
			s.retries++
		}
		s.mutex.Unlock()
		if stopped {
			abandonProcess(process)
			process.Wait()
			return false
		}
		// Ready が成功するまでは reconnecting のままである。これは新しい
		// shellなので、成功後にだけ前の続きではないことを伝える。
		s.observeProcess(StateReconnecting,
			"\r\n[sshc] 再接続しました。新しいシェルを開始しました。これより前の表示は切断前の記録です。\r\n")
		return true
	}
}

func (s *Session) publish(chunk []byte) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	_, _ = s.buffer.Write(chunk)
	for stream := range s.streams {
		// 複製するのは、この配列を次の読み取りが上書きするからである。
		copied := make([]byte, len(chunk))
		copy(copied, chunk)
		select {
		case stream.output <- copied:
		default:
			// 追いつけないアタッチは落とす。PTY は止めない。
			stream.dropped.Store(true)
			delete(s.streams, stream)
			stream.close()
		}
	}
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
	for stream := range s.streams {
		delete(s.streams, stream)
		stream.close()
	}
	if s.cleanup != nil {
		cleanup := s.cleanup
		s.cleanup = nil
		cleanup()
	}
}

// prepareManualReconnect は終了済みのSSHセッションを同じIDで再利用する。
// 呼び出し側が新しいProcessを確保する間はconnectingとして数え、同時実行と
// session上限の迂回を防ぐ。
func (s *Session) prepareManualReconnect() (func(context.Context, Size) (Process, error), Size, ExitInfo, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.exited == nil || s.reopen == nil || s.discarded {
		return nil, Size{}, ExitInfo{}, ErrReconnectUnavailable
	}
	previous := *s.exited
	s.process = nil
	s.exited = nil
	s.state = StateConnecting
	s.problem = ""
	s.reconnectView = nil
	s.retries = 0
	s.stopping = make(chan struct{})
	s.done = make(chan struct{})
	return s.reopen, s.size, previous, nil
}

func (s *Session) manualReconnectProblem(err error) string {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.reconnectError == nil {
		return "reconnect_failed"
	}
	_, problem := s.reconnectError(err)
	if problem == "" {
		return "reconnect_failed"
	}
	return problem
}

// failManualReconnect は接続前の終了状態へ戻す。新しく作ったdoneを閉じるため、
// engine停止も失敗した接続を待ち続けない。
func (s *Session) failManualReconnect(previous ExitInfo, problem string) {
	s.mutex.Lock()
	s.process = nil
	s.exited = &previous
	s.state = StateExited
	s.problem = problem
	s.reconnectView = nil
	done := s.done
	for stream := range s.streams {
		delete(s.streams, stream)
		stream.close()
	}
	s.mutex.Unlock()
	close(done)
}

// completeManualReconnect はcloseやshutdownが先行していなければ、新しいProcessを
// 同じsessionへ公開する。
func (s *Session) completeManualReconnect(process Process, started time.Time) bool {
	return s.completeProcessReplacement(process, started,
		"\r\n[sshc] 手動で再接続しました。新しいシェルを開始しました。これより前の表示は切断前の記録です。\r\n")
}

func (s *Session) completeProcessReplacement(process Process, started time.Time, successMessage string) bool {
	s.mutex.Lock()
	stopped := s.discarded
	select {
	case <-s.stopping:
		stopped = true
	default:
	}
	if stopped || s.exited != nil || s.state != StateConnecting {
		s.mutex.Unlock()
		return false
	}
	s.process = process
	s.started = started
	s.problem = ""
	s.reconnectView = nil
	s.mutex.Unlock()
	s.observeProcess(StateConnecting, successMessage)
	return true
}
