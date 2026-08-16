//go:build unix

package integration

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/creack/pty"
)

// terminalProcess は、本物の端末の向こうで走る sshc ひとつである。
//
// **パイプで代用できない。** vault の command はパスワードを求める前に
// 「これは端末か」を訊き、端末でなければ断る。その拒否こそ確かめたいものの
// ひとつなので、確かめる側は本物の端末を用意しなければならない。
type terminalProcess struct {
	command  *exec.Cmd
	terminal *os.File
	output   *lockedBuffer
	exited   chan error
	// drained は、端末から読み切ったことを表す。**command.Wait() は待ってくれ
	// ない。** あちらが待つのは子だけで、こちらが端末から写している goroutine は
	// 別に走っている。子が終わった瞬間に output を読むと、速く終わった子ほど
	// 空に見える——落ちるのは、正しく動いているものの方である。
	drained chan struct{}
}

func startOnTerminal(t *testing.T, home string, args ...string) *terminalProcess {
	t.Helper()
	command := exec.Command(binaryPath, args...)
	command.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"USERPROFILE=" + home,
		"TMPDIR=" + home,
		"TERM=dumb",
	}
	terminal, err := pty.Start(command)
	if err != nil {
		t.Fatal(err)
	}
	process := &terminalProcess{
		command:  command,
		terminal: terminal,
		output:   &lockedBuffer{},
		exited:   make(chan error, 1),
		drained:  make(chan struct{}),
	}
	go func() {
		defer close(process.drained)
		// 端末が閉じると Read は EIO で終わる。読み続けるのは、書いた側が
		// 埋まって止まらないためでもある。
		_, _ = io.Copy(process.output, terminal)
	}()
	go func() { process.exited <- command.Wait() }()
	t.Cleanup(func() {
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		_ = terminal.Close()
	})
	return process
}

// expect は、端末に指定の文字列が現れるまで待つ。
func (process *terminalProcess) expect(t *testing.T, want string, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if bytes.Contains([]byte(process.output.String()), []byte(want)) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%q never appeared on the terminal; saw:\n%s", want, process.output.String())
}

func (process *terminalProcess) typeLine(t *testing.T, line string) {
	t.Helper()
	if _, err := process.terminal.Write([]byte(line + "\n")); err != nil {
		t.Fatal(err)
	}
}

func (process *terminalProcess) wait(t *testing.T, within time.Duration) int {
	t.Helper()
	select {
	case err := <-process.exited:
		process.waitForOutput()
		if err == nil {
			return 0
		}
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatalf("wait: %v\n%s", err, process.output.String())
		}
		return exit.ExitCode()
	case <-time.After(within):
		t.Fatalf("still running after %s; saw:\n%s", within, process.output.String())
		return -1
	}
}

// waitForOutput は、端末から読み切るのを短く待つ。読み切れなくても進む
// ——そこで落とすと、確かめたいものではなく後片付けを検査することになる。
func (process *terminalProcess) waitForOutput() {
	select {
	case <-process.drained:
	case <-time.After(2 * time.Second):
	}
}

func (process *terminalProcess) running() bool {
	select {
	case err := <-process.exited:
		process.exited <- err
		return false
	default:
		return true
	}
}

// interrupt は Ctrl-C の一バイトを端末へ送る。信号を直接送らないのは、利用者が
// 押すのがキーだからである——端末の line discipline がそれを信号に変える経路
// ごと確かめたい。
func (process *terminalProcess) interrupt(t *testing.T) {
	t.Helper()
	if _, err := process.terminal.Write([]byte{3}); err != nil {
		t.Fatal(err)
	}
}
