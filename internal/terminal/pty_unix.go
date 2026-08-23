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

// UnixStarter は Unix の擬似端末を確保する。
type UnixStarter struct{}

func NewStarter() Starter { return UnixStarter{} }

func (UnixStarter) Start(ctx context.Context, command Command, size Size) (Process, error) {
	if command.Path == "" {
		return nil, errors.New("terminal: the program path is empty")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	child := exec.Command(command.Path, command.Arguments...)
	if command.Argv0 != "" {
		child.Args = append([]string{command.Argv0}, command.Arguments...)
	}
	child.Env = command.Env
	child.Dir = command.Dir
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

// ForceClose は子プロセスグループへ SIGKILL を送り、PTY を解放する。
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
