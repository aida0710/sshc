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
	outdated  bool
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

func (manager *fakeServiceManager) IsDefinitionOutdated() (bool, error) {
	return manager.outdated, nil
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
		engineStatus: (&fixedEngineStatus{answer: lockedVault}).read,
	}
	var stdout bytes.Buffer
	if code := runService(context.Background(), serviceRun{action: "status", yes: true, home: "/home/test", stdout: &stdout, stderr: io.Discard, dependencies: dependencies}); code != 0 {
		t.Fatalf("status code = %d", code)
	}
	if resolved != 0 {
		t.Fatalf("status resolved the executable %d times", resolved)
	}
	if code := runService(context.Background(), serviceRun{action: "install", yes: true, home: "/home/test", stdout: &stdout, stderr: io.Discard, dependencies: dependencies}); code != 0 {
		t.Fatalf("install code = %d", code)
	}
	if resolved != 1 || manager.installed != "/opt/sshc/bin/sshc" {
		t.Fatalf("resolved=%d installed=%q", resolved, manager.installed)
	}
	if !strings.Contains(stdout.String(), "sshc: "+vaultLockedAdvice+"\n") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestServiceStatusTellsToReinstallADefinitionFromAnOlderVersion(t *testing.T) {
	for _, state := range []serviceState{serviceActive, serviceInactive} {
		manager := &fakeServiceManager{state: state, outdated: true}
		dependencies := serviceDependencies{
			manager: func(string) (engineServiceManager, error) { return manager, nil },
		}
		var stdout bytes.Buffer
		code := runService(context.Background(), serviceRun{action: "status", home: "/home/test", stdout: &stdout, stderr: io.Discard, dependencies: dependencies})
		if code != 0 || !strings.HasSuffix(stdout.String(), "sshc: "+outdatedServiceDefinitionAdvice+"\n") {
			t.Fatalf("state=%v code=%d stdout=%q", state, code, stdout.String())
		}
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
		if code := runService(context.Background(), serviceRun{action: action, yes: true, home: "/home/test", stdout: io.Discard, stderr: &stderr, dependencies: dependencies}); code != 1 {
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
	code := runService(context.Background(), serviceRun{action: "install", yes: true, home: "/home/test", stdout: io.Discard, stderr: &stderr, dependencies: serviceDependencies{
		manager: func(string) (engineServiceManager, error) { return nil, errors.New("unsupported") },
		executable: func(context.Context) (string, error) {
			resolved = true
			return "", nil
		},
	}})
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
	if code := runService(context.Background(), serviceRun{action: "install", yes: false, home: "/home/test", stdout: &stdout, stderr: io.Discard, dependencies: dependencies}); code != 0 {
		t.Fatalf("install code = %d", code)
	}
	if manager.installed != "" || confirmed != 1 || !strings.Contains(stdout.String(), "canceled") {
		t.Fatalf("installed=%q confirmed=%d stdout=%q", manager.installed, confirmed, stdout.String())
	}
	if code := runService(context.Background(), serviceRun{action: "disable", yes: false, home: "/home/test", stdout: &stdout, stderr: io.Discard, dependencies: dependencies}); code != 0 {
		t.Fatalf("disable code = %d", code)
	}
	if confirmed != 2 {
		t.Fatalf("confirmations = %d", confirmed)
	}
}

func TestServiceInstallDoesNotAskToUnlockAPasswordlessVault(t *testing.T) {
	status := &fixedEngineStatus{answer: passwordlessVault}
	var stdout bytes.Buffer
	code := runService(context.Background(), serviceRun{action: "install", yes: true, home: "/home/test", stdout: &stdout, stderr: io.Discard, dependencies: serviceDependencies{
		manager:      func(string) (engineServiceManager, error) { return &fakeServiceManager{}, nil },
		executable:   func(context.Context) (string, error) { return "/opt/sshc/bin/sshc", nil },
		engineStatus: status.read,
	}})
	if code != 0 || stdout.String() != "sshc: install service using /opt/sshc/bin/sshc\nsshc: service installed and started\n" {
		t.Fatalf("code=%d stdout=%q", code, stdout.String())
	}
}
