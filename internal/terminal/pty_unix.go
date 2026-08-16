//go:build unix

package terminal

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// UnixStarter は、本物の擬似端末を確保する唯一の実装である。
//
// stdlib にこれを行う手段はない。golang.org/x/term が扱えるのは既存の fd の
// raw モードとサイズだけで、対を確保することはできない。creack/pty は darwin と
// linux の両方を覆う。
type UnixStarter struct{}

func NewStarter() Starter { return UnixStarter{} }

func (UnixStarter) Start(ctx context.Context, command Command, size Size) (Process, error) {
	if command.Path == "" {
		return nil, errors.New("terminal: the program path is empty")
	}
	// 確保が取り消されたなら、起こさない。ここで起こしてしまえば、誰も
	// 引き取らない子プロセスが残る。
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	child := exec.Command(command.Path, command.Arguments...)
	if command.Argv0 != "" {
		child.Args = append([]string{command.Argv0}, command.Arguments...)
	}
	child.Env = command.Env
	child.Dir = command.Dir
	// StartWithSize は setsid と TIOCSCTTY を立てる。子は自分のセッションの
	// リーダーになり、この PTY を制御端末として持つ。だから SIGHUP はその
	// プロセスグループ全体に届き、ssh が起こした何かが取り残されることはない。
	file, err := pty.StartWithSize(child, &pty.Winsize{Cols: size.Cols, Rows: size.Rows})
	if err != nil {
		return nil, err
	}
	return &unixProcess{file: file, child: child}, nil
}

type unixProcess struct {
	file  *os.File
	child *exec.Cmd
}

func (p *unixProcess) Read(b []byte) (int, error)  { return p.file.Read(b) }
func (p *unixProcess) Write(b []byte) (int, error) { return p.file.Write(b) }

func (p *unixProcess) Resize(size Size) error {
	return pty.Setsize(p.file, &pty.Winsize{Cols: size.Cols, Rows: size.Rows})
}

// Hangup は、子のプロセスグループ全体へ SIGHUP を送る。
//
// 負の pid を渡すのはグループを指すためであり、それは子が session leader だから
// 成立する。子ひとつに送ると、その子が起こしたものが親を失ったまま残る。
func (p *unixProcess) Hangup() error {
	if p.child.Process == nil {
		return nil
	}
	if err := syscall.Kill(-p.child.Process.Pid, syscall.SIGHUP); err != nil {
		// グループが既に消えているのは失敗ではない。求めていた状態そのものである。
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return err
	}
	return nil
}

// ForceClose は、子のプロセスグループへ SIGKILL を送り、PTY を手放す。
//
// SIGHUP と SIGTERM を無視する子が居る。**無視できない合図がひとつ要る。**
// PTY も閉じるのは、それが pump の読み取りを終わらせ、セッションの done を
// 閉じさせる唯一の手だからである。
func (p *unixProcess) ForceClose() error {
	var killErr error
	if p.child.Process != nil {
		if err := syscall.Kill(-p.child.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			killErr = err
		}
	}
	if err := p.file.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
		return errors.Join(killErr, err)
	}
	return killErr
}

func (p *unixProcess) Wait() ExitInfo {
	info := ExitInfo{At: time.Now()}
	err := p.child.Wait()
	if err == nil {
		return info
	}
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		info.Code = -1
		return info
	}
	info.Code = exitError.ExitCode()
	if status, ok := exitError.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		info.Signal = status.Signal().String()
	}
	return info
}

func (p *unixProcess) Close() error { return p.file.Close() }
