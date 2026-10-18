//go:build windows

package main

import "os/exec"

// The installer starts engines through the detached launcher or Task Scheduler.
// CommandContext cancellation only stops the foreground PowerShell invocation.
func configureUpdateCommand(_ *exec.Cmd) (killLeftovers func()) {
	return func() {}
}
