package sshclient

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ProxyCommand は、接続そのものを外部のプログラムに任せる設定である。
//
// **かつてここは断っていた。** 「このアプリケーションは接続のために何も実行
// しない」という判断で、~/.ssh/config は正本のまま残るのだから端末から ssh で
// 繋げばよい、という逃げ道が根拠だった。
//
// **その逃げ道は、この道具を使う理由そのものを削っていた。** ProxyCommand を
// 書いている接続先は珍しくない——`cloudflared access ssh`、`aws ssm`、
// 会社の bastion helper。それらが一覧に並んでいるのに押すと断られるなら、
// その人にとってこのアプリケーションは「半分の接続先しか扱えないもの」である。
//
// **いま実行する。ただし黙って実行しない。**
//
//   - 起こす綴りは、接続のたびに notice として出る（`sshc <alias>` は端末へ、
//     画面は接続の警告欄へ）。何が走るかを知らないまま走ることはない
//   - `-v` 以上では、起こす前にその綴りが端末に出る
//   - 走らせるのは利用者自身の ~/.ssh/config に書かれたものだけである。
//     ssh が読むのと同じ 1 行を、ssh と同じように解釈する
//
// **プログラムを起こす場所は数えられている。** internal/acceptance の
// TestOnlyTheNamedSubsystemsStartAProgram が一覧を持っており、ここはそこへ
// 理由と一緒に足した 4 つ目である。

// ErrProxyCommandThroughJump は、踏み台の向こうのホップが ProxyCommand を
// 持っている設定を断る。
//
// **そのプログラムはこの機械で走る。** 手前のホップの中ではない。つまり
// 「踏み台の向こうから見た接続先」へは届かず、走るのはこちらのネットワークに
// 居るプログラムである。**それは設定が言っていることと違う。**
var ErrProxyCommandThroughJump = errors.New(
	"a jump host reached through another connection cannot use ProxyCommand; the command would run on this machine")

// ErrProxyCommandWithJump は、ProxyJump と ProxyCommand を同時に書いた設定を断る。
//
// **OpenSSH も断る**（"inconsistent options: ProxyCommand+ProxyJump"）。
// どちらも「どうやってそこへ届くか」を決めるものなので、両方を書いた人は
// 二つの違う答えを書いている。
var ErrProxyCommandWithJump = errors.New("ProxyCommand and ProxyJump cannot both decide how to reach one host")

// proxyCommandGrace は、接続が終わったあとプログラムが自分で畳むのを待つ長さ。
//
// **待ってから殺す。** パイプを閉じれば普通のプログラムは終わる。終わらない
// ものだけが強制される。
const proxyCommandGrace = 2 * time.Second

// proxyCommandStderrLimit は、プログラムの stderr を覚えておく量である。
//
// **全部は覚えない。** 何時間も喋り続けるプログラムがあれば、それはこの
// プロセスのメモリになる。要るのは「なぜ繋がらなかったか」の最後の数行だけである。
const proxyCommandStderrLimit = 8 << 10

// startProxyCommand は、その綴りを起こし、その標準入出力を接続として返す。
func startProxyCommand(command string) (net.Conn, error) {
	name, arguments, err := interpreter(command)
	if err != nil {
		return nil, err
	}

	childStdin, ourWriter, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	ourReader, childStdout, err := os.Pipe()
	if err != nil {
		_ = childStdin.Close()
		_ = ourWriter.Close()
		return nil, err
	}

	complaints := &boundedBuffer{limit: proxyCommandStderrLimit}
	process := exec.Command(name, arguments...)
	process.Stdin = childStdin
	process.Stdout = childStdout
	process.Stderr = complaints

	if err := process.Start(); err != nil {
		for _, file := range []*os.File{childStdin, ourWriter, ourReader, childStdout} {
			_ = file.Close()
		}
		return nil, fmt.Errorf("ProxyCommand did not start: %w", err)
	}
	// **子に渡した端は、親が閉じなければならない。** 開いたままだと、子は
	// stdin の EOF を見ず、こちらは stdout の EOF を見ない——どちらも
	// 相手が終わるのを待ち続ける。
	_ = childStdin.Close()
	_ = childStdout.Close()

	return &commandConn{
		command:    command,
		process:    process,
		reader:     ourReader,
		writer:     ourWriter,
		complaints: complaints,
	}, nil
}

// commandConn は、起こしたプログラムの標準入出力をひとつの接続として見せる。
type commandConn struct {
	command    string
	process    *exec.Cmd
	reader     *os.File
	writer     *os.File
	complaints *boundedBuffer

	mutex       sync.Mutex
	readTimer   *time.Timer
	writeTimer  *time.Timer
	expiredRead bool
	expiredSend bool

	closeOnce sync.Once
	closeErr  error
}

func (c *commandConn) Read(b []byte) (int, error) {
	n, err := c.reader.Read(b)
	return n, c.translate(err, true)
}

func (c *commandConn) Write(b []byte) (int, error) {
	n, err := c.writer.Write(b)
	return n, c.translate(err, false)
}

// translate は、締め切りで畳んだことを「閉じた」ではなく「間に合わなかった」
// として返す。
//
// **区別が要る。** 相手が黙っているのと、こちらが畳んだのは違う出来事であり、
// 読む人が次にすることも違う。
func (c *commandConn) translate(err error, reading bool) error {
	if err == nil {
		return nil
	}
	c.mutex.Lock()
	expired := c.expiredRead
	if !reading {
		expired = c.expiredSend
	}
	c.mutex.Unlock()
	if expired && errors.Is(err, os.ErrClosed) {
		return os.ErrDeadlineExceeded
	}
	return err
}

// Close は接続を畳み、プログラムを終わらせる。
func (c *commandConn) Close() error {
	c.closeOnce.Do(func() {
		c.mutex.Lock()
		for _, timer := range []*time.Timer{c.readTimer, c.writeTimer} {
			if timer != nil {
				timer.Stop()
			}
		}
		c.mutex.Unlock()

		_ = c.writer.Close()
		_ = c.reader.Close()

		finished := make(chan error, 1)
		go func() { finished <- c.process.Wait() }()
		select {
		case <-finished:
		case <-time.After(proxyCommandGrace):
			_ = c.process.Process.Kill()
			<-finished
		}
	})
	return c.closeErr
}

// Complaints は、プログラムが標準エラーへ書いたものを返す。
//
// **繋がらなかった理由は、たいていここにしかない。** `ssh -W` は
// "Connection refused" をここへ書く。握手の失敗としてだけ見せると、読む人は
// 何が起きたのか分からない。
func (c *commandConn) Complaints() string { return c.complaints.String() }

func (c *commandConn) LocalAddr() net.Addr  { return proxyAddr{command: c.command} }
func (c *commandConn) RemoteAddr() net.Addr { return proxyAddr{command: c.command} }

func (c *commandConn) SetDeadline(t time.Time) error {
	if err := c.SetReadDeadline(t); err != nil {
		return err
	}
	return c.SetWriteDeadline(t)
}

// SetReadDeadline は、締め切りが来たら接続を畳む。
//
// **os.File の締め切りに任せない。** Windows の匿名パイプはそれを支えないので、
// あちらでだけ締め切りが効かない接続ができる。**効かない締め切りは、無い
// 締め切りより悪い** ——呼び出し側は掛けたつもりで待ち続ける。
func (c *commandConn) SetReadDeadline(t time.Time) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.readTimer = c.rearm(c.readTimer, t, func() {
		c.mutex.Lock()
		c.expiredRead = true
		c.mutex.Unlock()
		_ = c.Close()
	})
	return nil
}

func (c *commandConn) SetWriteDeadline(t time.Time) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.writeTimer = c.rearm(c.writeTimer, t, func() {
		c.mutex.Lock()
		c.expiredSend = true
		c.mutex.Unlock()
		_ = c.Close()
	})
	return nil
}

// rearm は、掛かっている時限を外し、必要なら掛け直す。mutex を握って呼ぶこと。
func (c *commandConn) rearm(timer *time.Timer, t time.Time, fire func()) *time.Timer {
	if timer != nil {
		timer.Stop()
	}
	if t.IsZero() {
		return nil
	}
	return time.AfterFunc(time.Until(t), fire)
}

// proxyAddr は、この接続の相手を名指す。
//
// **IP もポートも無い。** 相手はプログラムであり、どこへ繋がるかを知っているのは
// そのプログラムだけである。綴りをそのまま見せるのが、一番正確な答えである。
type proxyAddr struct{ command string }

func (proxyAddr) Network() string  { return "proxycommand" }
func (a proxyAddr) String() string { return a.command }

// boundedBuffer は、上限まで覚えて、その先を捨てる書き込み先である。
type boundedBuffer struct {
	limit int
	mutex sync.Mutex
	kept  []byte
}

func (b *boundedBuffer) Write(chunk []byte) (int, error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	if room := b.limit - len(b.kept); room > 0 {
		if len(chunk) > room {
			b.kept = append(b.kept, chunk[:room]...)
		} else {
			b.kept = append(b.kept, chunk...)
		}
	}
	// **捨てた分も書けたと答える。** 書けなかったと答えると os/exec は
	// そこで写しを止め、プログラム側の書き込みが詰まる。
	return len(chunk), nil
}

func (b *boundedBuffer) String() string {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return strings.TrimSpace(string(b.kept))
}

var _ io.Writer = (*boundedBuffer)(nil)
var _ net.Conn = (*commandConn)(nil)
