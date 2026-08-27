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
	id      string
	kind    Kind
	alias   string
	title   string
	started time.Time

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
	// Forwards は、このセッションが開いている転送である。
	Forwards []Forward
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
func (s *Session) Kind() Kind { return s.kind }

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

// Title は一覧に出す名前である。改名できるので、ロックの中で読む。
func (s *Session) Title() string {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.title
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
	return nil
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
	return view
}

// Snapshot は、いまスクロールバックに残っているバイト列を返す。
func (s *Session) Snapshot() []byte {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.buffer.Snapshot()
}

// Attach は、バッファの内容を先に返し、その後ライブの出力へ継ぐ。
func (s *Session) Attach() ([]byte, *Stream) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	replay := s.buffer.Snapshot()
	stream := &Stream{output: make(chan []byte, streamDepth)}
	if s.exited != nil {
		// 終了済みのセッションにライブの出力は無い。読めるものを渡してから閉じる。
		stream.close()
		return replay, stream
	}
	s.streams[stream] = true
	return replay, stream
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

// WriteCommand sends a command and Enter to the exact Process generation which
// was previewed. It shares inputMutex with WebSocket keystrokes, so bytes from
// the two sources cannot interleave inside this frame.
func (s *Session) WriteCommand(ctx context.Context, generation uint64, command string) error {
	if len(command) == 0 || len(command) > MaxCommandBytes {
		return ErrCommandTooLarge
	}
	payload := make([]byte, len(command)+1)
	copy(payload, command)
	payload[len(command)] = '\r'

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
func (s *Session) Resize(size Size) error {
	if !size.Valid() {
		return ErrInvalidSize
	}
	s.mutex.Lock()
	process, exited := s.process, s.exited
	s.mutex.Unlock()
	if exited != nil || process == nil {
		return ErrExited
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

const reconnectJitterMaxPercent = 120

// reconnectBackoff は、試みのあいだに置く間隔である。
var reconnectBackoff = []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second, 15 * time.Second}

// ReconnectWindow は指定回数の再接続待機時間の合計を返す。
func ReconnectWindow(attempts int) time.Duration {
	attempts = NormaliseReconnects(attempts)
	var total time.Duration
	for attempt := range attempts {
		total += reconnectBackoff[min(attempt, len(reconnectBackoff)-1)]
	}
	maximum := total * reconnectJitterMaxPercent / 100
	return ((maximum + time.Second - 1) / time.Second) * time.Second
}

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
		process, ready := s.process, s.ready
		s.mutex.Unlock()
		buffer := make([]byte, readChunk)
		for {
			read, err := process.Read(buffer)
			if read > 0 {
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

// reconnect は、落ちた輸送を繋ぎ直せたなら真を返す。
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
		reopen, size, attempt := s.reopen, s.size, s.retries
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
				s.publish([]byte("\r\n[sshc] 再接続の試行上限に達しました。\r\n"))
			}
			return false
		}

		wait := reconnectBackoff[min(attempt, len(reconnectBackoff)-1)]
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
			"\r\n[sshc] 接続が切れました。%d 秒後に繋ぎ直します（%d/%d）。\r\n",
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
			if forcer, ok := process.(forceCloser); ok {
				_ = forcer.ForceClose()
			}
			_ = process.Close()
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
			if forcer, ok := process.(forceCloser); ok {
				_ = forcer.ForceClose()
			}
			_ = process.Close()
			process.Wait()
			return false
		}
		// Ready が成功するまでは reconnecting のままである。これは新しい
		// shellなので、成功後にだけ前の続きではないことを伝える。
		s.observeProcess(StateReconnecting,
			"\r\n[sshc] 繋ぎ直しました。新しいシェルです。前の画面より上は、切れる前の記録です。\r\n")
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
	s.observeProcess(StateConnecting,
		"\r\n[sshc] 手動で繋ぎ直しました。新しいシェルです。前の画面より上は、切れる前の記録です。\r\n")
	return true
}
