//go:build !windows

package process

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// KillGroupOnCancel は、command の ctx が終わったとき、command が起動した子プロセスも
// 一緒に止めるようにする。waitDelay は、止めたあと出力のパイプが閉じるのを待つ上限である。
//
// ctx 付きで作った Cmd は、既定では command の直下のプロセスだけを止める。docker build が
// 起動した docker-buildx や、ログインシェルが起動したコマンドのような子プロセスが出力の
// パイプを持ったまま残ると、Wait はパイプが閉じるまで戻らない。command を新しい
// プロセスグループの先頭にして、グループごと SIGKILL で止める。
func KillGroupOnCancel(command *exec.Cmd, waitDelay time.Duration) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		if command.Process == nil {
			return os.ErrProcessDone
		}
		err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			// グループはもう無い。Wait には、止める前に終わったことを伝える。
			return os.ErrProcessDone
		}
		return err
	}
	command.WaitDelay = waitDelay
}

// KillSessionOnCancel は、KillGroupOnCancel と同じく ctx が終わったときに子プロセスごと
// 止めるが、command を新しいプロセスグループではなく新しい session の先頭にする。
//
// 呼び出し元の制御端末を継がせないためである。前面や tmux で動く engine の端末を継いだ
// 対話シェルは、job control のために端末を取りに行き、背景のプロセスグループとして
// SIGTTOU・SIGTTIN で止まる。session の先頭はプロセスグループの先頭でもあるので、
// グループごと止める Cancel はそのまま効く。
func KillSessionOnCancel(command *exec.Cmd, waitDelay time.Duration) {
	KillGroupOnCancel(command, waitDelay)
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
