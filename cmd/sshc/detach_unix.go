//go:build unix

package main

import (
	"os/exec"
	"syscall"
)

// detach は、起こしたプロセスを自分のセッションのリーダーにする。
//
// **親が死ぬときに道連れにしないためである。** 端末から起こされた場合、
// 同じプロセスグループに残っていると、その端末が閉じたときの SIGHUP を
// 一緒に受ける。
func detach(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
