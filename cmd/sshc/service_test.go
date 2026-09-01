package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

type fakeServiceManager struct {
	installed string
	state     serviceState
	removed   bool
	err       error
}

func (manager *fakeServiceManager) Install(_ context.Context, executable string) error {
	manager.installed = executable
	return manager.err
}

func (manager *fakeServiceManager) InstallPlan(executable string) (string, error) {
	return "install service using " + executable, nil
}

func (manager *fakeServiceManager) Status(context.Context) (serviceState, error) {
	return manager.state, manager.err
}

func (manager *fakeServiceManager) RestartIfActive(context.Context, string) (bool, error) {
	return manager.state == serviceActive, manager.err
}

func (manager *fakeServiceManager) Disable(context.Context) (bool, error) {
	return manager.removed, manager.err
}

func (manager *fakeServiceManager) DisablePlan() string { return "disable service" }

func TestRunServiceResolvesAStableExecutableOnlyForInstall(t *testing.T) {
	manager := &fakeServiceManager{}
	resolved := 0
	dependencies := serviceDependencies{
		manager: func(string) (engineServiceManager, error) { return manager, nil },
		executable: func(context.Context) (string, error) {
			resolved++
			return "/opt/sshc/bin/sshc", nil
		},
	}
	var stdout bytes.Buffer
	if code := runService(context.Background(), "status", true, "/home/test", &stdout, io.Discard, dependencies); code != 0 {
		t.Fatalf("status code = %d", code)
	}
	if resolved != 0 {
		t.Fatalf("status resolved the executable %d times", resolved)
	}
	if code := runService(context.Background(), "install", true, "/home/test", &stdout, io.Discard, dependencies); code != 0 {
		t.Fatalf("install code = %d", code)
	}
	if resolved != 1 || manager.installed != "/opt/sshc/bin/sshc" {
		t.Fatalf("resolved=%d installed=%q", resolved, manager.installed)
	}
	if !strings.Contains(stdout.String(), "vault is locked") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunServiceRefusesAnUnmanagedUnit(t *testing.T) {
	manager := &fakeServiceManager{state: serviceUnmanaged, err: errUnmanagedServiceUnit}
	dependencies := serviceDependencies{
		manager:    func(string) (engineServiceManager, error) { return manager, nil },
		executable: func(context.Context) (string, error) { return "/opt/sshc/bin/sshc", nil },
	}
	for _, action := range []string{"install", "disable"} {
		var stderr bytes.Buffer
		if code := runService(context.Background(), action, true, "/home/test", io.Discard, &stderr, dependencies); code != 1 {
			t.Fatalf("%s code = %d", action, code)
		}
		if !strings.Contains(stderr.String(), "not managed by sshc") {
			t.Fatalf("%s stderr = %q", action, stderr.String())
		}
	}
}

func TestRunServiceReportsAnUnsupportedPlatformBeforeResolvingTheExecutable(t *testing.T) {
	resolved := false
	var stderr bytes.Buffer
	code := runService(context.Background(), "install", true, "/home/test", io.Discard, &stderr, serviceDependencies{
		manager: func(string) (engineServiceManager, error) { return nil, errors.New("unsupported") },
		executable: func(context.Context) (string, error) {
			resolved = true
			return "", nil
		},
	})
	if code != 1 || resolved || !strings.Contains(stderr.String(), "unavailable") {
		t.Fatalf("code=%d resolved=%v stderr=%q", code, resolved, stderr.String())
	}
}

func TestRunServiceConfirmsMutatingActions(t *testing.T) {
	manager := &fakeServiceManager{state: serviceInactive, removed: true}
	confirmed := 0
	dependencies := serviceDependencies{
		manager:    func(string) (engineServiceManager, error) { return manager, nil },
		executable: func(context.Context) (string, error) { return "/opt/sshc/bin/sshc", nil },
		confirm: func(context.Context, string) (bool, error) {
			confirmed++
			return false, nil
		},
	}
	var stdout bytes.Buffer
	if code := runService(context.Background(), "install", false, "/home/test", &stdout, io.Discard, dependencies); code != 0 {
		t.Fatalf("install code = %d", code)
	}
	if manager.installed != "" || confirmed != 1 || !strings.Contains(stdout.String(), "canceled") {
		t.Fatalf("installed=%q confirmed=%d stdout=%q", manager.installed, confirmed, stdout.String())
	}
	if code := runService(context.Background(), "disable", false, "/home/test", &stdout, io.Discard, dependencies); code != 0 {
		t.Fatalf("disable code = %d", code)
	}
	if confirmed != 2 {
		t.Fatalf("confirmations = %d", confirmed)
	}
}
