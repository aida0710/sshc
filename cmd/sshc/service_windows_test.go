//go:build windows

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestWindowsServiceRejectsUnmanagedExecutablesWithThePowerShellCommand(t *testing.T) {
	_, err := managedInstallationExecutable(context.Background(), installation{manager: managerUnknown, executable: `C:\Downloads\sshc-windows-amd64.exe`}, nil)
	if err == nil || !strings.Contains(err.Error(), "PowerShell") || !strings.Contains(err.Error(), windowsInstallerCommand) {
		t.Fatalf("guidance=%v", err)
	}
	manager, scheduler := newTestTaskManager(t)
	var stderr bytes.Buffer
	code := runService(context.Background(), serviceRun{action: "install", yes: true, stdout: io.Discard, stderr: &stderr, dependencies: serviceDependencies{
		manager:    func(string) (engineServiceManager, error) { return manager, nil },
		executable: func(context.Context) (string, error) { return "", err },
	}})
	if code != exitFailure || len(scheduler.events) != 0 || !strings.Contains(stderr.String(), windowsInstallerCommand) {
		t.Fatalf("exit=%d events=%v stderr=%s", code, scheduler.events, stderr.String())
	}
}

func TestWindowsUpdatesStayDisabledEvenWithAPowerShellReceipt(t *testing.T) {
	dependencies := updateDependencies{
		executable: func() (string, error) {
			t.Fatal("update inspected installation before rejecting Windows")
			return "", nil
		},
		detect: func(string) (installation, error) { return installation{manager: managerPowerShell}, nil },
	}
	var stderr bytes.Buffer
	if code := runUpdate(context.Background(), updateRun{yes: true, stdout: io.Discard, stderr: &stderr, dependencies: dependencies}); code != exitFailure || !strings.Contains(stderr.String(), "cannot be updated automatically on Windows") {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if _, err := inspectWebUpdateInstallation(context.Background(), dependencies); err == nil || !strings.Contains(err.Error(), "update_windows_unsupported") {
		t.Fatalf("web update=%v", err)
	}
}

func TestWindowsReadinessRequiresTheRegisteredProcessExecutable(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyTaskEngineExecutable(os.Getpid(), executable); err != nil {
		t.Fatal(err)
	}
	if err := verifyTaskEngineExecutable(os.Getpid(), `C:\unrelated\sshc.exe`); !errors.Is(err, errEngineIdentityMismatch) {
		t.Fatalf("different executable=%v", err)
	}
	if err := verifyTaskEngineExecutable(0, executable); err == nil {
		t.Fatal("accepted inaccessible PID")
	}
}
