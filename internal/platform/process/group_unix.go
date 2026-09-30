//go:build !windows

package process

import (
	"errors"
	"os"
	"os/exec"
	"sync/atomic"
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
		return signalGroup(command.Process.Pid, syscall.SIGKILL)
	}
	command.WaitDelay = waitDelay
}

// TerminateGroupOnCancel は、KillGroupOnCancel と同じく command が起動した子プロセスも
// グループごと止めるが、ctx が終わったときに先に SIGTERM を送り、後始末をさせる。SIGKILL は
// trap で受けられないので、最初から送るとシェルスクリプトが置いた一時ファイルが残る。grace を
// 過ぎても command が終わらなければ、exec が command を SIGKILL で止める。
//
// 戻り値は Wait が戻った後に呼ぶ。取り消されていたときだけ、SIGTERM を無視してグループに
// 残ったものを SIGKILL で止め、command だけが終わって子プロセスが残ることのないようにする。
func TerminateGroupOnCancel(command *exec.Cmd, grace time.Duration) (killLeftovers func()) {
	var canceled atomic.Bool
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		if command.Process == nil {
			return os.ErrProcessDone
		}
		canceled.Store(true)
		return signalGroup(command.Process.Pid, syscall.SIGTERM)
	}
	command.WaitDelay = grace
	return func() {
		if !canceled.Load() || command.Process == nil {
			return
		}
		// グループに誰かが残っている間、その ID はほかのプロセスに再利用されない。グループが
		// 空なら ESRCH で何もしない。Wait が先頭を回収した直後に呼ぶので、空のグループの ID が
		// 既に別のプロセスへ割り当てられている余地はほぼない。
		_ = signalGroup(command.Process.Pid, syscall.SIGKILL)
	}
}

// signalGroup は、leader が先頭のプロセスグループ全体へ signal を送る。グループがもう
// 無ければ、Wait に止める前に終わったことを伝えるため os.ErrProcessDone を返す。
func signalGroup(leader int, signal syscall.Signal) error {
	err := syscall.Kill(-leader, signal)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
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
