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
