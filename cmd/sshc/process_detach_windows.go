//go:build windows

package main

import (
	"errors"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func configureDetachedProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS | windows.CREATE_BREAKAWAY_FROM_JOB}
}

func shouldRetryWithoutBreakaway(err error) bool { return errors.Is(err, windows.ERROR_ACCESS_DENIED) }
func clearProcessBreakaway(command *exec.Cmd) {
	command.SysProcAttr.CreationFlags &^= windows.CREATE_BREAKAWAY_FROM_JOB
}

func watchDetachedProcess(command *exec.Cmd) (*detachedProcess, error) {
	pid := command.Process.Pid
	process, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	_ = command.Process.Release()
	if err != nil {
		return nil, err
	}
	stop, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		windows.CloseHandle(process)
		return nil, err
	}
	exited := make(chan processExit, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		defer windows.CloseHandle(process)
		selected, err := windows.WaitForMultipleObjects([]windows.Handle{process, stop}, false, windows.INFINITE)
		if err != nil {
			exited <- processExit{code: -1, err: err}
			return
		}
		if selected == windows.WAIT_OBJECT_0 {
			var code uint32
			err := windows.GetExitCodeProcess(process, &code)
			exited <- processExit{code: int(code), err: err}
		}
	}()
	return &detachedProcess{pid: pid, exited: exited, release: func() {
		// Stop observing as soon as readiness is known; no process handle survives.
		_ = windows.SetEvent(stop)
		<-finished
		_ = windows.CloseHandle(stop)
	}}, nil
}
