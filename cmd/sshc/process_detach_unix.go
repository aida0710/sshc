//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

func configureDetachedProcess(command *exec.Cmd) {
	// Both the engine and update helper must survive their parent's terminal.
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func shouldRetryWithoutBreakaway(error) bool { return false }
func clearProcessBreakaway(*exec.Cmd)        {}

func watchDetachedProcess(command *exec.Cmd) (*detachedProcess, error) {
	exited := make(chan processExit, 1)
	go func() {
		err := command.Wait()
		exited <- processExit{code: command.ProcessState.ExitCode(), err: err}
	}()
	return &detachedProcess{pid: command.Process.Pid, exited: exited, release: func() {}}, nil
}
