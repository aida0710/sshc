//go:build windows

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"sshc/internal/enginelog"
	"sshc/internal/handoff"
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

func TestLogonTaskEngineMayTakeLongerToStartThanAServiceManagerGetsToAnswer(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	stateDir := t.TempDir()
	published := time.AfterFunc(serviceReadyTimeout+time.Second, func() {
		document := testHandoff(backgroundTestURL)
		document.PID = os.Getpid()
		if err := handoff.Write(stateDir, document); err != nil {
			t.Error(err)
		}
	})
	defer published.Stop()
	engine := taskEngine{stateDir: stateDir, client: backgroundTestClient(func(response http.ResponseWriter, _ *http.Request) {
		io.WriteString(response, validEngineStatus())
	})}
	if err := engine.wait(context.Background(), executable, func(context.Context) error { return nil }); err != nil {
		t.Fatalf("wait = %v", err)
	}
}

func TestLogonTaskEngineThatExitsIsReportedWithTheEndOfItsLog(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	stateDir := t.TempDir()
	log, err := enginelog.Open(stateDir, "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := log.WriteString("prepare this machine: the password vault is locked\n"); err != nil {
		t.Fatal(err)
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	engine := taskEngine{stateDir: stateDir, client: backgroundTestClient(func(http.ResponseWriter, *http.Request) {})}
	err = engine.wait(context.Background(), executable, func(context.Context) error { return errTaskEngineExited })
	if !errors.Is(err, errTaskEngineExited) || !strings.Contains(err.Error(), "the password vault is locked") {
		t.Fatalf("wait = %v", err)
	}
}
