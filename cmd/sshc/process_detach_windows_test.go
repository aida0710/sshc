//go:build windows

package main

import (
	"errors"
	"os/exec"
	"testing"

	"golang.org/x/sys/windows"
)

func isDetachedTestProcess() bool {
	getConsoleWindow := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleWindow")
	window, _, _ := getConsoleWindow.Call()
	return window == 0
}

func TestWindowsDetachmentRetriesOnlyAJobAccessDenial(t *testing.T) {
	command := exec.Command("sshc", "engine")
	configureDetachedProcess(command)
	if command.SysProcAttr.CreationFlags != windows.CREATE_NEW_PROCESS_GROUP|windows.DETACHED_PROCESS|windows.CREATE_BREAKAWAY_FROM_JOB {
		t.Fatalf("creation flags = %x", command.SysProcAttr.CreationFlags)
	}
	clearProcessBreakaway(command)
	if command.SysProcAttr.CreationFlags != windows.CREATE_NEW_PROCESS_GROUP|windows.DETACHED_PROCESS {
		t.Fatalf("retry flags = %x", command.SysProcAttr.CreationFlags)
	}
	if !shouldRetryWithoutBreakaway(errors.Join(errors.New("CreateProcess"), windows.ERROR_ACCESS_DENIED)) {
		t.Fatal("access denied did not permit retry")
	}
	if shouldRetryWithoutBreakaway(windows.ERROR_FILE_NOT_FOUND) || shouldRetryWithoutBreakaway(nil) {
		t.Fatal("retried an unrelated error")
	}
}
