//go:build linux

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshc/internal/storage"
)

type fakeServiceCommandRunner struct {
	calls   [][]string
	results []serviceCommandResult
	err     error
}

func (runner *fakeServiceCommandRunner) Run(_ context.Context, arguments ...string) (serviceCommandResult, error) {
	runner.calls = append(runner.calls, append([]string(nil), arguments...))
	if runner.err != nil {
		return serviceCommandResult{}, runner.err
	}
	if len(runner.results) == 0 {
		return serviceCommandResult{}, nil
	}
	result := runner.results[0]
	runner.results = runner.results[1:]
	return result, nil
}

func testLinuxServiceManager(t *testing.T, runner serviceCommandRunner) *linuxServiceManager {
	t.Helper()
	return &linuxServiceManager{
		home:   t.TempDir(),
		runner: runner,
		files:  storage.OSFileSystem{},
	}
}

func TestLinuxServiceInstallWritesAManagedUnitAndRestartsIt(t *testing.T) {
	runner := &fakeServiceCommandRunner{}
	manager := testLinuxServiceManager(t, runner)
	if err := manager.Install(context.Background(), `/opt/brew $share%/bin/sshc`); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(manager.unitPath())
	if err != nil {
		t.Fatal(err)
	}
	unit := string(contents)
	for _, want := range []string{
		serviceUnitMarker,
		`ExecStart="/opt/brew $$share%%/bin/sshc" engine`,
		"Restart=on-failure",
		"SuccessExitStatus=130",
	} {
		if !strings.Contains(unit, want) {
			t.Errorf("unit does not contain %q:\n%s", want, unit)
		}
	}
	info, err := os.Stat(manager.unitPath())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("unit permissions = %o, want 600", info.Mode().Perm())
	}
	wantCalls := []string{
		"--user daemon-reload",
		"--user enable sshc.service",
		"--user restart sshc.service",
	}
	if len(runner.calls) != len(wantCalls) {
		t.Fatalf("systemctl calls = %#v", runner.calls)
	}
	for index, want := range wantCalls {
		if got := strings.Join(runner.calls[index], " "); got != want {
			t.Errorf("call %d = %q, want %q", index, got, want)
		}
	}
}

func TestLinuxServiceDoesNotOverwriteOrRemoveAnUnmanagedUnit(t *testing.T) {
	runner := &fakeServiceCommandRunner{}
	manager := testLinuxServiceManager(t, runner)
	if err := os.MkdirAll(filepath.Dir(manager.unitPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	manual := []byte("[Service]\nExecStart=/custom/sshc engine\n")
	if err := os.WriteFile(manager.unitPath(), manual, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := manager.Install(context.Background(), "/opt/sshc/bin/sshc"); !errors.Is(err, errUnmanagedServiceUnit) {
		t.Fatalf("install error = %v", err)
	}
	if _, err := manager.Disable(context.Background()); !errors.Is(err, errUnmanagedServiceUnit) {
		t.Fatalf("disable error = %v", err)
	}
	contents, err := os.ReadFile(manager.unitPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != string(manual) || len(runner.calls) != 0 {
		t.Fatalf("manual unit changed=%v calls=%#v", string(contents) != string(manual), runner.calls)
	}
}

func TestLinuxServiceStatusDistinguishesManagedStates(t *testing.T) {
	runner := &fakeServiceCommandRunner{results: []serviceCommandResult{{ExitCode: 0}, {ExitCode: 3}}}
	manager := testLinuxServiceManager(t, runner)
	if state, err := manager.Status(context.Background()); err != nil || state != serviceAbsent {
		t.Fatalf("absent status = %v, %v", state, err)
	}
	if err := os.MkdirAll(filepath.Dir(manager.unitPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	unit, err := systemdUnit("/opt/sshc/bin/sshc")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manager.unitPath(), []byte(unit), 0o600); err != nil {
		t.Fatal(err)
	}
	if state, err := manager.Status(context.Background()); err != nil || state != serviceActive {
		t.Fatalf("active status = %v, %v", state, err)
	}
	if state, err := manager.Status(context.Background()); err != nil || state != serviceInactive {
		t.Fatalf("inactive status = %v, %v", state, err)
	}
}

func TestLinuxServiceRestartTouchesOnlyAnActiveManagedUnit(t *testing.T) {
	runner := &fakeServiceCommandRunner{results: []serviceCommandResult{{ExitCode: 0}, {ExitCode: 0}}}
	manager := testLinuxServiceManager(t, runner)
	if err := os.MkdirAll(filepath.Dir(manager.unitPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	unit, err := systemdUnit("/opt/sshc/bin/sshc")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manager.unitPath(), []byte(unit), 0o600); err != nil {
		t.Fatal(err)
	}
	restarted, err := manager.RestartIfActive(context.Background())
	if err != nil || !restarted {
		t.Fatalf("restart = %v, %v", restarted, err)
	}
	if len(runner.calls) != 2 || strings.Join(runner.calls[1], " ") != "--user restart sshc.service" {
		t.Fatalf("calls = %#v", runner.calls)
	}

	manual := []byte("[Service]\nExecStart=/custom/sshc engine\n")
	if err := os.WriteFile(manager.unitPath(), manual, 0o600); err != nil {
		t.Fatal(err)
	}
	restarted, err = manager.RestartIfActive(context.Background())
	if err != nil || restarted || len(runner.calls) != 2 {
		t.Fatalf("unmanaged restart = %v, %v calls=%#v", restarted, err, runner.calls)
	}
}

func TestLinuxServiceDisableRemovesOnlyManagedUnit(t *testing.T) {
	runner := &fakeServiceCommandRunner{}
	manager := testLinuxServiceManager(t, runner)
	if err := os.MkdirAll(filepath.Dir(manager.unitPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	unit, err := systemdUnit("/opt/sshc/bin/sshc")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manager.unitPath(), []byte(unit), 0o600); err != nil {
		t.Fatal(err)
	}
	removed, err := manager.Disable(context.Background())
	if err != nil || !removed {
		t.Fatalf("disable = %v, %v", removed, err)
	}
	if _, err := os.Stat(manager.unitPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unit still exists: %v", err)
	}
	if got := strings.Join(runner.calls[0], " "); got != "--user disable --now sshc.service" {
		t.Fatalf("first call = %q", got)
	}
	if got := strings.Join(runner.calls[1], " "); got != "--user daemon-reload" {
		t.Fatalf("second call = %q", got)
	}
}

func TestSystemdUnitRejectsUnsafeExecutablePaths(t *testing.T) {
	for _, executable := range []string{"relative/sshc", "/opt/sshc\nExecStart=/bin/sh", "/opt/sshc\x00"} {
		if _, err := systemdUnit(executable); err == nil {
			t.Errorf("systemdUnit(%q) accepted an unsafe path", executable)
		}
	}
}
