package sshclient

import (
	"context"
	"io"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"sshc/internal/terminal"
)

// TermName は、リモートへ伝える端末の種別である。
//
// 画面を描くのは xterm.js なので、その能力に合った名前を送る。ここが実際と
// 違うと、vim も less も間違った制御列を送ってくる。
const TermName = "xterm-256color"

// Session は、開かれている SSH のセッションひとつである。
//
// terminal.Process を満たすが、**プロセスを持たない。** PTY も確保しない——
// SSH のチャンネルがそのまま端末である。
type Session struct {
	// input は、端末から打たれたバイト列である。
	//
	// 握手のあいだは問いの答えとして読まれ、シェルが始まったあとは
	// リモートの stdin へ流れる。切り替えは要らない——順番に起きるからである。
	input *InputBuffer
	// reader と writer は、端末へ出ていくバイト列である。握手のあいだの
	// 問いも、シェルの出力も、同じ道を通る。
	reader *io.PipeReader
	writer *io.PipeWriter

	// cancel は、まだ握手の途中なら、それごと止める。
	//
	// **閉じたセッションが繋ぎ続けてはならない。** 届かないアドレスへの接続は
	// タイムアウトまで生き、その間ずっと goroutine とソケットを保持する。
	cancel context.CancelFunc

	mutex   sync.Mutex
	remote  *ssh.Session
	size    terminal.Size
	closers []io.Closer

	// forwarded は、この接続の上に開いた転送である。セッションの寿命に縛る
	// ——**閉じ忘れるものを増やさない。**
	forwarded forwards

	exit      terminal.ExitInfo
	done      chan struct{}
	closeOnce sync.Once
	doneOnce  sync.Once
}

func newSession(size terminal.Size, cancel context.CancelFunc) *Session {
	reader, writer := io.Pipe()
	return &Session{
		input: NewInputBuffer(), reader: reader, writer: writer,
		size: size, cancel: cancel, done: make(chan struct{}),
	}
}

// Forwards は、このセッションが開いている転送を報告する。
func (s *Session) Forwards() []terminal.Forward { return s.forwarded.list() }

// Prompter は、この端末のストリームへ問いを出す。
func (s *Session) Prompter() Prompter {
	return StreamPrompter{Out: s.writer, In: s.input}
}

func (s *Session) Read(b []byte) (int, error) { return s.reader.Read(b) }

// Write は、打たれたバイト列を受け取る。
//
// 握手のあいだは問いの答えになり、シェルが始まったあとはリモートへ流れる。
// **決して待たない**——問いが出ていない間に打たれた文字で WebSocket の
// 読み手が止まると、その接続全体が固まる。
func (s *Session) Write(b []byte) (int, error) { return s.input.Write(b) }

// Resize は window-change を送る。まだチャンネルが無ければ、要求された大きさを
// 覚えておいて pty-req に使う。
func (s *Session) Resize(size terminal.Size) error {
	s.mutex.Lock()
	s.size = size
	remote := s.remote
	s.mutex.Unlock()
	if remote == nil {
		return nil
	}
	return remote.WindowChange(int(size.Rows), int(size.Cols))
}

// Hangup は、向こうに終わってほしいという意思である。
//
// SIGHUP は無い。**プロセスが無いからである。** 同じ意図をチャンネルを
// 閉じることで表す。
func (s *Session) Hangup() error {
	s.mutex.Lock()
	remote := s.remote
	s.mutex.Unlock()
	if remote != nil {
		_ = remote.Signal(ssh.SIGHUP)
		return remote.Close()
	}
	return s.Close()
}

// Wait は、このセッションが終わった理由を返す。
func (s *Session) Wait() terminal.ExitInfo {
	<-s.done
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.exit
}

// Close は、繋いだものを手前まで含めて手放す。
func (s *Session) Close() error {
	s.closeOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		s.mutex.Lock()
		remote, closers := s.remote, s.closers
		s.mutex.Unlock()
		s.forwarded.close()
		if remote != nil {
			_ = remote.Close()
		}
		// 手前のホップは奥から順に閉じる。手前を先に閉じると、奥の接続は
		// その上に載っているので、閉じ方が「切断」になる。
		for index := len(closers) - 1; index >= 0; index-- {
			_ = closers[index].Close()
		}
		_ = s.input.Close()
		_ = s.writer.Close()
		s.finish(terminal.ExitInfo{Code: -1, At: time.Now()})
	})
	return nil
}

// ForceClose は、graceful な終わり方を待たずに輸送そのものを断つ。
//
// **殺すプロセスは無い。** Unix の PTY で SIGKILL と PTY のクローズが果たす役割
// ——pump の読み取りを終わらせ、セッションの done を閉じさせること——を、ここでは
// 手前のホップを閉じることが果たす。
//
// Close と違い、リモートのチャンネルより**先に**輸送を閉じる。順序がこのメソッドの
// 全部である。応答を返さない相手に対しては、チャンネルを閉じる書き込みそのものが
// 返らないことがあり、そうなると締切に間に合わせるために呼ばれたものが締切を越える。
// 輸送が先に死んでいれば、そのあとの Close は何にも待たされない。
func (s *Session) ForceClose() error {
	if s.cancel != nil {
		s.cancel()
	}
	s.mutex.Lock()
	closers := s.closers
	s.mutex.Unlock()
	// 奥から順に閉じる。Close と同じ向きである。
	for index := len(closers) - 1; index >= 0; index-- {
		_ = closers[index].Close()
	}
	return s.Close()
}

// finish は、終了の理由を一度だけ記録する。
func (s *Session) finish(info terminal.ExitInfo) {
	s.doneOnce.Do(func() {
		s.mutex.Lock()
		s.exit = info
		s.mutex.Unlock()
		close(s.done)
	})
}

// fail は、接続できなかった理由を端末へ書いて終わらせる。
//
// **セッションは残す。** 接続できなかった理由が読めるのはそこだけである。
func (s *Session) fail(reason string) {
	_, _ = io.WriteString(s.writer, "\r\n"+reason+"\r\n")
	s.finish(terminal.ExitInfo{Code: 255, At: time.Now()})
	_ = s.writer.Close()
	_ = s.input.Close()

	s.mutex.Lock()
	closers := s.closers
	s.mutex.Unlock()
	for index := len(closers) - 1; index >= 0; index-- {
		_ = closers[index].Close()
	}
}

// attach は、開いたチャンネルをこのセッションへ結び付ける。
func (s *Session) attach(remote *ssh.Session, closers []io.Closer) terminal.Size {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.remote = remote
	s.closers = closers
	return s.size
}

// run は、シェルが終わるまで待ち、その理由を記録する。
func (s *Session) run(remote *ssh.Session, keepAlive func()) {
	if keepAlive != nil {
		go keepAlive()
	}
	err := remote.Wait()
	info := terminal.ExitInfo{At: time.Now()}
	switch typed := err.(type) {
	case nil:
	case *ssh.ExitError:
		info.Code = typed.ExitStatus()
		info.Signal = typed.Signal()
	default:
		// 接続が落ちた。終了コードではないので、そう分かる形で残す。
		info.Code = -1
		_, _ = io.WriteString(s.writer, "\r\n"+err.Error()+"\r\n")
	}
	s.finish(info)
	s.forwarded.close()
	_ = s.writer.Close()
	_ = s.input.Close()

	s.mutex.Lock()
	closers := s.closers
	s.mutex.Unlock()
	for index := len(closers) - 1; index >= 0; index-- {
		_ = closers[index].Close()
	}
}

// keepAliveLoop は、生きているかをリモートへ尋ね続ける。
//
// 応答しないまま count 回過ぎた接続は落ちたものとして閉じる。これが無いと、
// 経路が黙って死んだ接続が、TCP のタイムアウトまで生きているふりをする。
func keepAliveLoop(client *ssh.Client, interval time.Duration, count int, done <-chan struct{}) func() {
	if interval <= 0 {
		return nil
	}
	if count <= 0 {
		count = 3
	}
	return func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		missed := 0
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if _, _, err := client.SendRequest("keepalive@openssh.com", true, nil); err != nil {
					missed++
					if missed >= count {
						_ = client.Close()
						return
					}
					continue
				}
				missed = 0
			}
		}
	}
}
