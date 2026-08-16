package terminal

import (
	"sync"
	"sync/atomic"
	"time"
)

// streamDepth は、ひとつのアタッチが溜め込めるチャンクの数である。
//
// これを超えたクライアントは読んでいない。リングバッファは上書きを続け、
// WebSocket 側は落とす。PTY は止めない——止めれば、読まないタブひとつが
// リモート側のプログラムを凍らせてしまう。
const streamDepth = 256

// readChunk は、PTY から一度に読むバイト数である。
const readChunk = 32 << 10

// Stream は、ひとつのアタッチである。
//
// 落とされたことと、セッションが終わったことは別の事実なので、チャンネルが
// 閉じたあとに Dropped がそれを言う。前者では同じ ID へ繋ぎ直せるし、
// 後者では終了の理由が読める。
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

	// done は pump が終わったことを示す。テストと停止処理だけが待つ。
	done chan struct{}
}

// View は、一覧に出すためのセッションひとつ分である。
//
// 中身は決して運ばない。スクロールバックは WebSocket からしか出ていかない。
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

func (s *Session) ID() string    { return s.id }
func (s *Session) Kind() Kind    { return s.kind }
func (s *Session) Alias() string { return s.alias }

// Title は一覧に出す名前である。改名できるので、ロックの中で読む。
func (s *Session) Title() string {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.title
}

// Rename は一覧に出す名前を変える。
//
// 変えるのは表示だけである。ssh の相手も、走っているプロセスも、この
// セッションの識別子も動かない。名前が要るのは、同じ相手へ複数本開いたときに
// 行が見分けられなくなるからであって、それ以外の意味は持たせない。
//
// 終了したセッションも改名できる。読むために残してある行なので、印を付ける
// 価値はそこにもある。
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

// Exit は、終了していればその理由を返し、生きていれば nil を返す。
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
//
// title と exited はどちらもロックの中で読む。改名は接続中にも起きるので、
// 直接読むとその瞬間の一覧取得と競合する。
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
	// **開いていることが見えないまま開かない。** 転送を報告できる Process だけが
	// 答える——ローカルシェルは何も転送しない。
	if forwarder, ok := s.process.(Forwarder); ok {
		view.Forwards = forwarder.Forwards()
	}
	return view
}

// Snapshot は、いまスクロールバックに残っているバイト列を返す。
//
// これが出ていく先は WebSocket だけである。一覧の応答も、ログも、ディスクも
// これを受け取らない。
func (s *Session) Snapshot() []byte {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.buffer.Snapshot()
}

// Attach は、バッファの内容を先に返し、その後ライブの出力へ継ぐ。
//
// 複製と登録が同じロックの中で起きることが、この二つの間に落ちるバイトが
// 無いことの理由である。
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

// Detach は、そのアタッチを取り除く。セッションは死なない。
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
	process, exited := s.process, s.exited
	s.mutex.Unlock()
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

// Hangup は子プロセスへ SIGHUP を送る。終了そのものは pump が観測する。
func (s *Session) Hangup() error {
	s.mutex.Lock()
	process, exited := s.process, s.exited
	s.mutex.Unlock()
	if exited != nil || process == nil {
		return nil
	}
	return process.Hangup()
}

// closeStreams は、アタッチしているものをすべて外す。
//
// 停止のときに呼ぶ。プロセスが応答しなくても、画面側は待たされずに済む。
// セッションそのものは終わらない——終わったことを言うのは pump だけである。
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
//
// 持っていなければ何もしない。**待たない。** 呼び出し側は締切に間に合わせる
// ためにこれを呼んでおり、graceful な Hangup が返らないことこそが理由である。
func (s *Session) forceClose() error {
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
func (s *Session) pump(now func() time.Time) {
	defer close(s.done)
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
	s.finish(info)
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
