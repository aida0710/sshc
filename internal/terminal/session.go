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

	mutex   sync.Mutex
	buffer  *Ring
	streams map[*Stream]bool
	process Process
	exited  *ExitInfo
	cleanup func()

	reopen  func(ctx context.Context, size Size) (Process, error)
	size    Size
	retries int
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

// View は、一覧に出すためのセッションひとつ分である。
type View struct {
	ID      string
	Kind    Kind
	Alias   string
	Title   string
	Started time.Time
	Exited  *ExitInfo
	// Forwards は、このセッションが開いている転送である。
	Forwards []Forward
}

func (s *Session) ID() string { return s.id }
func (s *Session) Kind() Kind { return s.kind }

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

func (s *Session) Live() bool { return s.Exit() == nil }

// View は一覧に出すための写しである。
func (s *Session) View() View {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	view := View{
		ID: s.id, Kind: s.kind, Alias: s.alias, Title: s.title,
		Started: s.started,
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
	s.mutex.Lock()
	process, exited, reopening := s.process, s.exited, s.reopen != nil
	s.mutex.Unlock()
	if exited == nil && process == nil && reopening {
		// 繋ぎ直しのあいだの打鍵は捨てる。溜めない。
		return len(p), nil
	}
	if exited != nil || process == nil {
		return 0, ErrExited
	}
	return process.Write(p)
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
		return nil
	}
	return forcer.ForceClose()
}

// pump は PTY を読み、バッファへ書き、アタッチしているものへ配る。
const MaxReconnects = 5

// reconnectBackoff は、試みのあいだに置く間隔である。
var reconnectBackoff = []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second, 15 * time.Second}

// ReconnectWindow は指定回数の再接続待機時間の合計を返す。
func ReconnectWindow(attempts int) time.Duration {
	attempts = NormaliseReconnects(attempts)
	var total time.Duration
	for attempt := range attempts {
		total += reconnectBackoff[min(attempt, len(reconnectBackoff)-1)]
	}
	return total
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
	for {
		buffer := make([]byte, readChunk)
		for {
			read, err := s.process.Read(buffer)
			if read > 0 {
				s.publish(buffer[:read])
			}
			if err != nil {
				break
			}
		}
		info := s.process.Wait()
		if info.At.IsZero() {
			info.At = now()
		}
		_ = s.process.Close()

		if !s.reconnect(info) {
			s.finish(info)
			return
		}
	}
}

// reconnect は、落ちた輸送を繋ぎ直せたなら真を返す。
func (s *Session) stopReconnecting() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	select {
	case <-s.stopping:
	default:
		close(s.stopping)
	}
}

func (s *Session) reconnect(info ExitInfo) bool {
	s.mutex.Lock()
	reopen, size, attempt := s.reopen, s.size, s.retries
	stopping := s.exited != nil
	s.mutex.Unlock()

	// 閉じられたことを、待ちに入る前に見る。
	select {
	case <-s.stopping:
		stopping = true
	default:
	}

	limit := MaxReconnects
	if s.attempts != nil {
		limit = NormaliseReconnects(s.attempts())
	}

	if reopen == nil || !info.Lost() || stopping || attempt >= limit {
		if reopen != nil && info.Lost() && limit > 0 && attempt >= limit {
			s.publish([]byte("\r\n[sshc] 再接続の試行上限に達しました。\r\n"))
		}
		return false
	}

	wait := reconnectBackoff[min(attempt, len(reconnectBackoff)-1)]
	if s.delay != nil {
		wait = s.delay(attempt)
	}
	s.publish([]byte(fmt.Sprintf(
		"\r\n[sshc] 接続が切れました。%d 秒後に繋ぎ直します（%d/%d）。\r\n",
		int(wait.Seconds()), attempt+1, limit)))

	// 待っているあいだ、打つ先は無い。 閉じた相手へ書きに行かせない。
	s.mutex.Lock()
	s.process = nil
	s.mutex.Unlock()

	select {
	case <-time.After(wait):
	case <-s.stopping:
		return false
	}

	process, err := reopen(context.Background(), size)
	if err != nil {
		s.mutex.Lock()
		s.retries++
		s.mutex.Unlock()
		s.publish([]byte("\r\n[sshc] " + err.Error() + "\r\n"))
		return s.reconnect(info)
	}

	s.mutex.Lock()
	s.process = process
	s.retries++
	s.mutex.Unlock()
	// これは新しいシェルである。 前のものが残っていると思わせない。
	s.publish([]byte("\r\n[sshc] 繋ぎ直しました。新しいシェルです。前の画面より上は、切れる前の記録です。\r\n"))
	return true
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
