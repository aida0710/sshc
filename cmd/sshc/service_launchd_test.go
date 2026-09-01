//go:build !windows

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshc/internal/app"
	"sshc/internal/handoff"
	"sshc/internal/storage"
)

type fakeLaunchdCommandRunner struct {
	calls   [][]string
	results []launchdCommandResult
	err     error
}

func (runner *fakeLaunchdCommandRunner) Run(_ context.Context, arguments ...string) (launchdCommandResult, error) {
	runner.calls = append(runner.calls, append([]string(nil), arguments...))
	if runner.err != nil {
		return launchdCommandResult{}, runner.err
	}
	if len(runner.results) == 0 {
		return launchdCommandResult{}, nil
	}
	result := runner.results[0]
	runner.results = runner.results[1:]
	return result, nil
}

func testLaunchdServiceManager(t *testing.T, runner launchdCommandRunner) *launchdServiceManager {
	t.Helper()
	return &launchdServiceManager{
		home:   t.TempDir(),
		uid:    501,
		runner: runner,
		files:  storage.OSFileSystem{},
		waitReady: func(context.Context, string, int, launchdCommandRunner) error {
			return nil
		},
		lock: func() (func() error, error) {
			return func() error { return nil }, nil
		},
	}
}

func TestLaunchdServiceInstallWritesAPlistAndBootstrapsIt(t *testing.T) {
	runner := &fakeLaunchdCommandRunner{results: []launchdCommandResult{{ExitCode: 113, Output: []byte("Could not find service")}}}
	manager := testLaunchdServiceManager(t, runner)
	executable := "/opt/sshc & tools/bin/sshc"
	if err := manager.Install(context.Background(), executable); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(manager.plistPath())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{launchdPlistMarker, "/opt/sshc &amp; tools/bin/sshc", "<string>engine</string>", "<key>RunAtLoad</key>"} {
		if !strings.Contains(string(contents), want) {
			t.Errorf("plist does not contain %q:\n%s", want, contents)
		}
	}
	info, err := os.Stat(manager.plistPath())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("plist permissions = %o, want 600", info.Mode().Perm())
	}
	if len(runner.calls) != 2 || strings.Join(runner.calls[1], " ") != "bootstrap gui/501 "+manager.plistPath() {
		t.Fatalf("launchctl calls = %#v", runner.calls)
	}
}

func TestLaunchdServiceDoesNotTouchAnUnmanagedPlist(t *testing.T) {
	runner := &fakeLaunchdCommandRunner{}
	manager := testLaunchdServiceManager(t, runner)
	if err := os.MkdirAll(filepath.Dir(manager.plistPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	manual := []byte("<plist><dict><key>Label</key><string>custom</string></dict></plist>")
	if err := os.WriteFile(manager.plistPath(), manual, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := manager.Install(context.Background(), "/opt/sshc/bin/sshc"); !errors.Is(err, errUnmanagedServiceUnit) {
		t.Fatalf("install error = %v", err)
	}
	if _, err := manager.Disable(context.Background()); !errors.Is(err, errUnmanagedServiceUnit) {
		t.Fatalf("disable error = %v", err)
	}
	contents, err := os.ReadFile(manager.plistPath())
	if err != nil || !bytes.Equal(contents, manual) || len(runner.calls) != 0 {
		t.Fatalf("manual plist=%q err=%v calls=%#v", contents, err, runner.calls)
	}
}

func TestLaunchdServiceStatusDistinguishesManagedStates(t *testing.T) {
	runner := &fakeLaunchdCommandRunner{results: []launchdCommandResult{{ExitCode: 0, Output: []byte("pid = 4242\n")}, {ExitCode: 113, Output: []byte("Could not find service")}}}
	manager := testLaunchdServiceManager(t, runner)
	if state, err := manager.Status(context.Background()); err != nil || state != serviceAbsent {
		t.Fatalf("absent status = %v, %v", state, err)
	}
	plist, err := launchdPlist("/opt/sshc/bin/sshc", manager.home)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(manager.plistPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manager.plistPath(), []byte(plist), 0o600); err != nil {
		t.Fatal(err)
	}
	if state, err := manager.Status(context.Background()); err != nil || state != serviceActive {
		t.Fatalf("active status = %v, %v", state, err)
	}
	if state, err := manager.Status(context.Background()); err != nil || state != serviceInactive {
		t.Fatalf("inactive status = %v, %v", state, err)
	}
}

func TestLaunchdServiceRestartAndDisableTouchOnlyTheManagedAgent(t *testing.T) {
	runner := &fakeLaunchdCommandRunner{results: []launchdCommandResult{{ExitCode: 0, Output: []byte("pid = 4242\n")}}}
	manager := testLaunchdServiceManager(t, runner)
	plist, err := launchdPlist("/opt/sshc/bin/sshc", manager.home)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(manager.plistPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manager.plistPath(), []byte(plist), 0o600); err != nil {
		t.Fatal(err)
	}
	restarted, err := manager.RestartIfActive(context.Background(), "/opt/sshc/bin/sshc")
	if err != nil || !restarted {
		t.Fatalf("restart = %v, %v", restarted, err)
	}
	if got := strings.Join(runner.calls[1], " "); got != "kickstart -k gui/501/"+launchdServiceLabel {
		t.Fatalf("restart call = %q", got)
	}

	runner.calls = nil
	runner.results = []launchdCommandResult{{ExitCode: 0}}
	removed, err := manager.Disable(context.Background())
	if err != nil || !removed {
		t.Fatalf("disable = %v, %v", removed, err)
	}
	if got := strings.Join(runner.calls[1], " "); got != "bootout gui/501/"+launchdServiceLabel {
		t.Fatalf("disable call = %q", got)
	}
	if _, err := os.Stat(manager.plistPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("plist still exists: %v", err)
	}
}

func TestLaunchdReadinessRequiresTheLaunchdPIDAndStatusAPI(t *testing.T) {
	secret, err := handoff.Mint(bytes.NewReader(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get(handoff.HeaderName) != secret {
			http.Error(writer, "forbidden", http.StatusForbidden)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"owner":"engine","version":"test","protocolVersion":1,"vault":false,"unlocked":false,"sessions":0}`)
	}))
	defer server.Close()
	home := t.TempDir()
	document := handoff.Handoff{SchemaVersion: handoff.SchemaVersion, URL: server.URL, Secret: secret, Owner: handoff.OwnerEngine, PID: 4242, Version: "test", ProtocolVersion: handoff.ProtocolVersion}
	if err := handoff.Write(app.HandoffDir(home), document); err != nil {
		t.Fatal(err)
	}
	runner := &fakeLaunchdCommandRunner{results: []launchdCommandResult{{Output: []byte("state = running\n\tpid = 4242\n")}}}
	if err := waitForLaunchdServiceReady(context.Background(), home, 501, runner); err != nil {
		t.Fatal(err)
	}
}

func TestLaunchdPlistRejectsUnsafePaths(t *testing.T) {
	for _, executable := range []string{"relative/sshc", "/opt/sshc\n/bin", "/opt/sshc\x00"} {
		if _, err := launchdPlist(executable, "/Users/test"); err == nil {
			t.Errorf("launchdPlist(%q) accepted an unsafe path", executable)
		}
	}
}
