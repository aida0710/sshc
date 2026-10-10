package sshclient

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"sshc/internal/connectionlog"
	"sshc/internal/terminal"
)

// Session は、開かれている SSH のセッションひとつである。
//
// terminal.Process を満たすが、プロセスを持たない。PTY も確保しない。
// SSH のチャンネルがそのまま端末である。
type Session struct {
	// input は、端末から打たれたバイト列である。
	//
	// 握手のあいだは問いの結果として読まれ、シェルが始まったあとは
	// リモートの stdin へ流れる。切り替えは要らない。順番に起きるからである。
	input *InputBuffer
	// reader と writer は、端末へ出ていくバイト列である。握手のあいだの
	// 問いも、シェルの出力も、同じ道を通る。
	reader *io.PipeReader
	writer *io.PipeWriter

	// cancel は、まだ握手の途中なら、それごと止める。
	//
	// 閉じたセッションが繋ぎ続けてはならない。届かないアドレスへの接続は
	// タイムアウトまで goroutine とソケットを保持する。
	cancel context.CancelFunc

	mutex    sync.Mutex
	closed   bool
	remote   *ssh.Session
	client   *ssh.Client
	size     terminal.Size
	closers  []io.Closer
	progress terminal.ConnectionProgress
	// ptyReady は、リモートが pty-req を受け入れたかどうか。remote はチャンネルが
	// 開いた時点で入るが、PTY が無い間の window-change を OpenSSH の sshd は
	// 後から来る pty-req の寸法で上書きする。それまでは寸法を覚えるだけにする。
	ptyReady bool
	// windowMutex は、window-change の送信を一本にする。送るたびに最新の size を
	// 読むので、最後に送ったものが必ず最新の寸法になる。
	windowMutex sync.Mutex

	// forwarded は、この接続上の転送。セッション終了時にまとめて閉じる。
	forwarded forwards
	// trace は、この接続の接続ログである。端末へ出す先も、進捗を置く先も
	// このセッションなので、接続処理はここから受け取る。
	trace *tracer
	// closingTrace は、シェルが終わるときの接続ログを、出力ではなく closingLog へ書く。
	closingTrace *tracer
	closingLog   closingLog

	exit      terminal.ExitInfo
	done      chan struct{}
	ready     chan struct{}
	readyErr  error
	closeOnce sync.Once
	doneOnce  sync.Once
	readyOnce sync.Once
}

func newSession(size terminal.Size, cancel context.CancelFunc) *Session {
	reader, writer := io.Pipe()
	input := NewInputBuffer()
	input.enablePromptGate()
	return &Session{
		input: input, reader: reader, writer: writer,
		size: size, cancel: cancel, done: make(chan struct{}), ready: make(chan struct{}),
	}
}

// Forwards は、このセッションが開いている転送を報告する。
func (s *Session) Forwards() []terminal.Forward { return s.forwarded.list() }

// StartForward opens one loopback-only local, remote or SOCKS5 forward on the existing
// authenticated transport.
func (s *Session) StartForward(kind, listenPort, destination string) (terminal.Forward, error) {
	var (
		spec ForwardSpec
		err  error
	)
	switch kind {
	case terminal.ForwardLocal:
		spec, err = ParseLocalForward(listenPort + " " + destination)
	case terminal.ForwardRemote:
		spec, err = ParseRemoteForward(listenPort + " " + destination)
	case terminal.ForwardDynamic:
		if destination != "" {
			return terminal.Forward{}, terminal.ErrInvalidForward
		}
		spec, err = ParseDynamicForward(listenPort)
	default:
		return terminal.Forward{}, terminal.ErrInvalidForward
	}
	if err != nil {
		return terminal.Forward{}, terminal.ErrInvalidForward
	}
	s.mutex.Lock()
	client, closed := s.client, s.closed
	s.mutex.Unlock()
	if client == nil || closed {
		return terminal.Forward{}, terminal.ErrNotConnected
	}
	// 一時転送の成否は呼び出し元へ同期的に返す。端末の出力 pipe へも書くと、
	// WebSocket の読者がまだ付いていない場面で StartForward 自体が止まり得る。
	return s.forwarded.startTemporary(client, spec)
}

func (s *Session) StopForward(id string) error { return s.forwarded.stop(id) }

// Prompter は、この端末のストリームへ問いを出す。
func (s *Session) Prompter() Prompter {
	return StreamPrompter{
		Out: s.writer, In: s.input,
		begin: s.input.beginPrompt,
		end:   s.input.endPrompt,
	}
}

// AwaitingPrompt reports whether Ready前の入力が、いま表示した認証promptへの
// 回答として受理される。terminal registryはそれ以外の先行入力を捨てる。
func (s *Session) AwaitingPrompt() bool { return s.input.awaitingPrompt() }

// ConnectionProgress returns a snapshot of the current SSH connection step.
// The terminal registry polls this while the asynchronous handshake is still
// running, so ProxyJump failures can be attributed to the correct hop.
func (s *Session) ConnectionProgress() terminal.ConnectionProgress {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.progress
}

func (s *Session) setProgress(progress terminal.ConnectionProgress) {
	s.mutex.Lock()
	s.progress = progress
	s.mutex.Unlock()
}

// Ready は、認証とリモートのシェルの起動が終わると閉じる。起動時のコマンドは
// ここを待つので、認証の問いへの答えとして流れない。何人でも待てる。
func (s *Session) Ready() <-chan struct{} { return s.ready }

// ReadyErr は、Ready が閉じた理由を返す。nil なら使える状態になった。
func (s *Session) ReadyErr() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.readyErr
}

func (s *Session) markReady(err error) {
	s.readyOnce.Do(func() {
		if err == nil {
			s.input.markUsable()
		}
		s.mutex.Lock()
		s.readyErr = err
		s.mutex.Unlock()
		close(s.ready)
	})
}

func (s *Session) Read(b []byte) (int, error) { return s.reader.Read(b) }

// Announce は、接続ログの設定に関係なく出す行を、このセッションのターミナルへ書く。
//
// 書き先の接続ログは接続の途中で決まるので、Ready が成功を返した後に呼ぶ。出力は
// ターミナルが読むまで進まないので、読み手と同じ goroutine からは呼ばない。
func (s *Session) Announce(message string) { s.trace.announce("%s", message) }

// Write は、打たれたバイト列を受け取る。
//
// 握手のあいだは問いの結果になり、シェルが始まったあとはリモートへ流れる。
// 決して待たない。問いが出ていない間に打たれた文字で WebSocket の
// 読み手が止まると、その接続全体が固まる。
func (s *Session) Write(b []byte) (int, error) { return s.input.Write(b) }

// WriteExact enqueues a complete broadcast frame without the silent overflow
// semantics used for ordinary keystrokes.
func (s *Session) WriteExact(ctx context.Context, b []byte) error {
	return s.input.WriteExact(ctx, b)
}

// Resize は window-change を送る。まだ PTY が無ければ、要求された大きさを
// 覚えておいて pty-req に使う。
func (s *Session) Resize(size terminal.Size) error {
	s.mutex.Lock()
	s.size = size
	s.mutex.Unlock()
	return s.sendWindowChange()
}

// currentSize は、いま覚えている寸法を返す。pty-req はこれを送る直前に読む。
func (s *Session) currentSize() terminal.Size {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.size
}

// markPtyReady は、pty-req が受け入れられたことを記録する。pty-req を送った
// あいだに Resize が来ていれば、その寸法を window-change で送り直す。
func (s *Session) markPtyReady(requested terminal.Size) error {
	s.mutex.Lock()
	s.ptyReady = true
	latest := s.size
	s.mutex.Unlock()
	if latest == requested {
		return nil
	}
	return s.sendWindowChange()
}

// sendWindowChange は、PTY があれば最新の寸法を window-change で送る。
func (s *Session) sendWindowChange() error {
	s.windowMutex.Lock()
	defer s.windowMutex.Unlock()
	s.mutex.Lock()
	remote, size, ready := s.remote, s.size, s.ptyReady
	s.mutex.Unlock()
	if !ready || remote == nil {
		return nil
	}
	return remote.WindowChange(int(size.Rows), int(size.Cols))
}

// Hangup はリモートへ SIGHUP を送り、SSH チャンネルを閉じる。
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
		s.markReady(context.Canceled)
		if s.cancel != nil {
			s.cancel()
		}
		s.mutex.Lock()
		s.closed = true
		remote, closers := s.remote, s.closers
		s.client = nil
		s.mutex.Unlock()
		// Remote listener cancellation needs a reply. Closing transport first
		// releases that wait even when the server has stopped answering requests.
		closeAll(closers)
		s.forwarded.close()
		if remote != nil {
			_ = remote.Close()
		}
		_ = s.input.Close()
		_ = s.writer.Close()
		// 自分から閉じたときも、シェルが終わったのではないので輸送の断絶として
		// 残す。繋ぎ直さないのは、閉じる前に再接続を止める呼び出し側の役目である。
		s.finish(terminal.ExitInfo{Code: terminal.ExitCodeUnknown, TransportLost: true, At: time.Now()})
	})
	return nil
}

// ForceClose は、リモートの応答を待たずに輸送を先に閉じる。
// チャンネルの Close 自体がブロックする場合にも停止期限を守れるようにする。
func (s *Session) ForceClose() error {
	if s.cancel != nil {
		s.cancel()
	}
	s.mutex.Lock()
	closers := s.closers
	s.mutex.Unlock()
	closeAll(closers)
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
// セッションは残す。接続できなかった理由が読めるのはそこだけである。
// シェルの終わり方と違い、ExitInfo.Notice ではなく出力へ書く。呼ばれるのは Ready の前だけで、
// シェルが動いていないので代替画面は無く、出力へ書けば接続ログの行との順番も保てる。
func (s *Session) fail(reason error) {
	if reason == nil {
		reason = errors.New("ssh connection failed")
	}
	s.markReady(reason)
	_, _ = io.WriteString(s.writer, "\r\n"+terminalNewlines(reason.Error())+"\r\n")
	s.finish(terminal.ExitInfo{Code: RemoteFailureExit, At: time.Now()})
	_ = s.writer.Close()
	_ = s.input.Close()

	s.mutex.Lock()
	closers := s.closers
	s.mutex.Unlock()
	closeAll(closers)
	s.forwarded.close()
}

// terminalNewlines は、文の中の改行を端末の改行（CRLF）にする。LF のままだと、
// 次の行が行頭へ戻らない。
func terminalNewlines(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\n", "\r\n")
}

// attach は、開いた輸送と、用意できていればチャンネルをこのセッションへ結び付ける。
func (s *Session) attach(remote *ssh.Session, closers []io.Closer) bool {
	s.mutex.Lock()
	if s.closed {
		s.mutex.Unlock()
		if remote != nil {
			_ = remote.Close()
		}
		closeAll(closers)
		return false
	}
	s.remote = remote
	s.closers = closers
	s.mutex.Unlock()
	return true
}

func (s *Session) attachClient(client *ssh.Client) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.closed {
		return false
	}
	s.client = client
	return true
}

// run は、シェルが終わるまで待ち、その理由を記録する。
func (s *Session) run(remote *ssh.Session, client *ssh.Client, keepAlive keepAliveSettings) {
	started := s.trace.now()
	if loop := keepAliveLoop(client, keepAlive); loop != nil {
		go loop()
	}
	err := remote.Wait()
	describeSessionExit(s.closingTrace, err, s.trace.since(started))
	s.finish(s.exitInfo(err, client, keepAlive))
	_ = s.writer.Close()
	_ = s.input.Close()

	s.mutex.Lock()
	closers := s.closers
	s.mutex.Unlock()
	closeAll(closers)
	s.forwarded.close()
}

// exitStatusMissingNotice は、終了コードを送らずにチャンネルを閉じるサーバーで
// シェルが終わったときに、ターミナルへ書く行である。ExitInfo.Notice で返す。
const exitStatusMissingNotice = "\r\n[sshc] サーバーが終了コードを送らずにセッションを閉じました。\r\n"

// exitInfo は、Wait の結果を終わり方に直す。シェルが終わるときの接続ログは、
// 起きた順に終わり方の文より前へ置く。
func (s *Session) exitInfo(err error, client *ssh.Client, keepAlive keepAliveSettings) terminal.ExitInfo {
	info := terminal.ExitInfo{At: time.Now()}
	var exit *ssh.ExitError
	var missing *ssh.ExitMissingError
	switch {
	case err == nil:
	case errors.As(err, &exit):
		info.Code = exit.ExitStatus()
		info.Signal = exit.Signal()
	case errors.As(err, &missing) && s.transportAnswers(client, keepAlive):
		// exit-status の送信は RFC 4254 で SHOULD であり、送らないサーバーもある。
		// Wait の結果だけでは輸送の断絶と区別できないので、輸送に尋ねて確かめる。
		// 応答があるならシェルが終わったのであり、繋ぎ直してはならない。
		// 終了コードは、OpenSSH の ssh と同じく 255 にする。
		info.Code = RemoteFailureExit
		info.Notice = exitStatusMissingNotice
	default:
		// 接続が落ちた。終了コードではないので、そう分かる形で残す。
		info.Code = terminal.ExitCodeUnknown
		info.TransportLost = true
		info.Notice = "\r\n" + err.Error() + "\r\n"
	}
	info.Notice = s.closingLog.before(info.Notice)
	return info
}

// transportAnswers は、SSH の輸送が keepalive に応答するかを返す。
//
// 輸送が落ちていれば、x/crypto/ssh は応答の道を閉じているので、すぐ失敗する。
// 応答が遅い相手には、keepalive の設定で切断と判断するまでの時間だけ待つ。
func (s *Session) transportAnswers(client *ssh.Client, keepAlive keepAliveSettings) bool {
	s.closingTrace.say(connectionlog.Detailed, "終了コードが届かないままチャンネルが閉じました。SSH接続が応答するか確かめます。")
	err := keepAliveReply(client, keepAlive.replyLimit(), keepAlive.done)
	if err != nil {
		s.closingTrace.say(connectionlog.Detailed, "SSH接続が応答しません：%v", err)
		return false
	}
	return true
}
