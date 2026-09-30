//go:build !windows

package commandconn

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// separateGroup は、起動するプログラムを自分のプロセスグループに分けるかを返す。
//
// 制御端末を持つプロセス（端末から実行した `sshc ssh`）では分けない。分けると、
// ProxyCommand が /dev/tty で尋ねたとき（`ssh -W` の踏み台のパスワード）に、
// 背景のグループとして SIGTTIN で止まる。制御端末を持たない engine では分けて、
// 閉じるときに子孫ごと止める。テストは決まった答えに差し替える。
var separateGroup = func() bool { return !hasControllingTerminal() }

// hasControllingTerminal は、このプロセスが制御端末を持つかを返す。持たなければ
// /dev/tty は開けない（ENXIO）。
func hasControllingTerminal() bool {
	terminal, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return false
	}
	_ = terminal.Close()
	return true
}

// prepareTree は、Start の前に、子孫をまとめて止められる起動の仕方にする。
func prepareTree(process *exec.Cmd) {
	if !separateGroup() {
		return
	}
	if process.SysProcAttr == nil {
		process.SysProcAttr = &syscall.SysProcAttr{}
	}
	process.SysProcAttr.Setpgid = true
}

// adoptTree は、起動したプログラムの木を返す。
func adoptTree(process *exec.Cmd) (processTree, error) {
	grouped := process.SysProcAttr != nil && process.SysProcAttr.Setpgid
	return groupTree{process: process.Process, grouped: grouped}, nil
}

// groupTree は、プロセスグループでまとめた木である。
type groupTree struct {
	process *os.Process
	grouped bool
}

func (tree groupTree) kill() error {
	if !tree.grouped {
		return tree.process.Kill()
	}
	// 負の pid はプロセスグループを指す。グループの id は起動したプログラムの pid である。
	err := syscall.Kill(-tree.process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

func (groupTree) release() {}
