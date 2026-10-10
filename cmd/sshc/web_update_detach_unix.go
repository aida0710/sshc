//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

func configureDetachedUpdateProcess(command *exec.Cmd) {
	// The helper must survive the terminal/session that owns a foreground engine.
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
