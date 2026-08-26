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

// ProxyCommand は、ssh_config に記載された外部コマンドの標準入出力を SSH 接続に使う。
// 実行前にコマンドを利用者へ表示する。

// ErrProxyCommandThroughJump は、踏み台の向こうのホップが ProxyCommand を
// 持っている設定を断る。
//
// コマンドはローカルで実行されるため、踏み台から到達する後続ホップには適用できない。
var ErrProxyCommandThroughJump = errors.New(
	"a jump host reached through another connection cannot use ProxyCommand; the command would run on this machine")

// ErrProxyCommandWithJump は、ProxyJump と ProxyCommand を同時に書いた設定を断る。
//
// 両方とも接続経路を指定するため、sshc は曖昧な設定を拒否する。
var ErrProxyCommandWithJump = errors.New("ProxyCommand and ProxyJump cannot both decide how to reach one host")

// proxyCommandGrace は、パイプを閉じてからプロセスを強制終了するまでの猶予時間。
const proxyCommandGrace = 2 * time.Second

// proxyCommandStderrLimit は、診断用に保持する stderr の最大サイズ。
const proxyCommandStderrLimit = 8 << 10

// startProxyCommand は、その表記を起動し、その標準入出力を接続として返す。
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
	configureProxyCommandProcess(process, command)
	process.Stdin = childStdin
	process.Stdout = childStdout
	process.Stderr = complaints

	if err := process.Start(); err != nil {
		for _, file := range []*os.File{childStdin, ourWriter, ourReader, childStdout} {
			_ = file.Close()
		}
		return nil, fmt.Errorf("ProxyCommand did not start: %w", err)
	}
	// 親側で不要なパイプ端を閉じ、EOF が伝播するようにする。
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

// commandConn は、起動したプログラムの標準入出力をひとつの接続として見せる。
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
// 相手側の終了とローカルの締め切りを区別し、後者は deadline error として返す。
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
			// Windows の cmd.exe は子プロセスが継承した pipe を保持していると、
			// Kill 後も Wait が返らないことがある。Close は接続終了処理なので、
			// 外部コマンドの不作法によって無期限に止めない。
			select {
			case <-finished:
			case <-time.After(proxyCommandGrace):
				c.closeErr = errors.New("ProxyCommand did not stop after it was killed")
			}
		}
	})
	return c.closeErr
}

// Complaints は、接続失敗の診断に使う ProxyCommand の標準エラーを返す。
func (c *commandConn) Complaints() string { return c.complaints.String() }

func (c *commandConn) LocalAddr() net.Addr  { return proxyAddr{command: c.command} }
func (c *commandConn) RemoteAddr() net.Addr { return proxyAddr{command: c.command} }

func (c *commandConn) SetDeadline(t time.Time) error {
	if err := c.SetReadDeadline(t); err != nil {
		return err
	}
	return c.SetWriteDeadline(t)
}

// SetReadDeadline は、OS 間で同じ動作にするため、期限到達時に接続を閉じる。
// Windows の匿名パイプは os.File の deadline をサポートしない。
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
// IP もポートも無い。相手はプログラムであり、どこへ繋がるかを知っているのは
// そのプログラムだけである。表記をそのまま見せるのが、一番正確な結果である。
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
	// 捨てた分も書けたと返す。書けなかったと返すと os/exec は
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
