// Package commandconn は、起動したプログラムの標準入出力をひとつの接続として見せる。
//
// ProxyCommand と、VPN コンテナの中継（docker exec）が使う。どちらも、プログラムが
// 標準入出力で運ぶバイト列を、SSH の輸送としてそのまま使う。
package commandconn

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// stopGrace は、パイプを閉じてからプロセスを強制終了するまでの猶予時間。
const stopGrace = 2 * time.Second

// complaintsLimit は、診断用に保持する標準エラーの最大サイズ。
const complaintsLimit = 8 << 10

// Start は、process を起動し、その標準入出力を接続として返す。
//
// process の Stdin、Stdout、Stderr はここで決めるので、呼び出し側は設定しない。
// name は、接続の相手として見せる表記である（ProxyCommand の行など）。
func Start(process *exec.Cmd, name string) (*Conn, error) {
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

	complaints := &boundedBuffer{limit: complaintsLimit}
	process.Stdin = childStdin
	process.Stdout = childStdout
	process.Stderr = complaints

	if err := process.Start(); err != nil {
		for _, file := range []*os.File{childStdin, ourWriter, ourReader, childStdout} {
			_ = file.Close()
		}
		return nil, err
	}
	// 親側で不要なパイプ端を閉じ、EOF が伝播するようにする。
	_ = childStdin.Close()
	_ = childStdout.Close()

	conn := &Conn{
		name:       name,
		process:    process,
		reader:     ourReader,
		writer:     ourWriter,
		complaints: complaints,
		exited:     make(chan struct{}),
	}
	go conn.reap()
	return conn, nil
}

// Conn は、起動したプログラムの標準入出力である。
type Conn struct {
	name       string
	process    *exec.Cmd
	reader     *os.File
	writer     *os.File
	complaints *boundedBuffer

	// exited は、プログラムが終わって回収できると閉じる。exitErr はその結果である。
	exited  chan struct{}
	exitErr error

	mutex       sync.Mutex
	readTimer   *time.Timer
	writeTimer  *time.Timer
	expiredRead bool
	expiredSend bool

	closeOnce sync.Once
	closeErr  error
}

// reap は、プログラムの終わりを待って回収する。
//
// 起動した直後から待つ。Close を待たずに回収しておけば、途中で終わったプログラムが
// 残らず、終わり方（ExitError）を接続の側から読める。
func (c *Conn) reap() {
	c.exitErr = c.process.Wait()
	close(c.exited)
}

func (c *Conn) Read(b []byte) (int, error) {
	n, err := c.reader.Read(b)
	return n, c.translate(err, true)
}

func (c *Conn) Write(b []byte) (int, error) {
	n, err := c.writer.Write(b)
	return n, c.translate(err, false)
}

// CloseWrite は、送る側が終わったことをプログラムへ伝える。
func (c *Conn) CloseWrite() error { return c.writer.Close() }

// translate は、締め切りで畳んだことを「閉じた」ではなく「間に合わなかった」
// として返す。
//
// 相手側の終了とローカルの締め切りを区別し、後者は deadline error として返す。
func (c *Conn) translate(err error, reading bool) error {
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
func (c *Conn) Close() error {
	c.closeOnce.Do(func() {
		c.mutex.Lock()
		for _, timer := range []*time.Timer{c.readTimer, c.writeTimer} {
			if timer != nil {
				timer.Stop()
			}
		}
		c.mutex.Unlock()

		_ = c.writer.Close()
		// Windows の匿名パイプは、別 goroutine が同期 ReadFile で待っていると
		// Close 自体がその読み取りの終了まで待つ。ここで直に閉じると、その先の
		// process.Kill へ進めず、プログラムと SSH の双方が相手の終了を待つ。
		// 読み取り側だけを別 goroutine で閉じ、プロセスを止める猶予と上限を
		// 必ず実行できるようにする。
		go func() { _ = c.reader.Close() }()

		select {
		case <-c.exited:
		case <-time.After(stopGrace):
			_ = c.process.Process.Kill()
			// Windows の cmd.exe は子プロセスが継承した pipe を保持していると、
			// Kill 後も Wait が返らないことがある。Close は接続終了処理なので、
			// 外部コマンドの不作法によって無期限に止めない。
			select {
			case <-c.exited:
			case <-time.After(stopGrace):
				c.closeErr = fmt.Errorf("%s did not stop after it was killed", c.name)
			}
		}
	})
	return c.closeErr
}

// Exited は、プログラムが終わって回収されると閉じる。
func (c *Conn) Exited() <-chan struct{} { return c.exited }

// ExitErr は、終わったプログラムの終わり方を返す。Exited が閉じる前は nil。
func (c *Conn) ExitErr() error {
	select {
	case <-c.exited:
		return c.exitErr
	default:
		return nil
	}
}

// Complaints は、プログラムが標準エラーへ書いたものを返す。接続失敗の診断に使う。
func (c *Conn) Complaints() string { return c.complaints.String() }

func (c *Conn) LocalAddr() net.Addr  { return Addr{Name: c.name} }
func (c *Conn) RemoteAddr() net.Addr { return Addr{Name: c.name} }

func (c *Conn) SetDeadline(t time.Time) error {
	if err := c.SetReadDeadline(t); err != nil {
		return err
	}
	return c.SetWriteDeadline(t)
}

// SetReadDeadline は、OS 間で同じ動作にするため、期限到達時に接続を閉じる。
// Windows の匿名パイプは os.File の deadline をサポートしない。
func (c *Conn) SetReadDeadline(t time.Time) error {
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

func (c *Conn) SetWriteDeadline(t time.Time) error {
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
func (c *Conn) rearm(timer *time.Timer, t time.Time, fire func()) *time.Timer {
	if timer != nil {
		timer.Stop()
	}
	if t.IsZero() {
		return nil
	}
	return time.AfterFunc(time.Until(t), fire)
}

// Addr は、この接続の相手を名指す。
//
// IP もポートも無い。相手はプログラムであり、どこへ繋がるかを知っているのは
// そのプログラムだけである。表記をそのまま見せるのが、一番正確な結果である。
type Addr struct{ Name string }

func (Addr) Network() string  { return "command" }
func (a Addr) String() string { return a.Name }

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
	// 捨てた分も書けたと返す。書けなかったと返すと os/exec は
	// そこで写しを止め、プログラム側の書き込みが詰まる。
	return len(chunk), nil
}

func (b *boundedBuffer) String() string {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return strings.TrimSpace(string(b.kept))
}

var _ net.Conn = (*Conn)(nil)
