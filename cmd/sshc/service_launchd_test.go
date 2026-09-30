//go:build !windows

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
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
	results []serviceCommandResult
	err     error
}

func (runner *fakeLaunchdCommandRunner) Run(_ context.Context, arguments ...string) (serviceCommandResult, error) {
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

func testLaunchdServiceManager(t *testing.T, runner serviceCommandRunner) *launchdServiceManager {
	t.Helper()
	return &launchdServiceManager{
		home:   t.TempDir(),
		uid:    501,
		runner: runner,
		files:  storage.OSFileSystem{},
		waitReady: func(context.Context, *launchdServiceManager) error {
			return nil
		},
		lock: func() (func() error, error) {
			return func() error { return nil }, nil
		},
	}
}

func TestLaunchdServiceInstallWritesAPlistAndBootstrapsIt(t *testing.T) {
	runner := &fakeLaunchdCommandRunner{results: []serviceCommandResult{{ExitCode: 113, Output: []byte("Could not find service")}}}
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

// systemd の Restart=on-failure と同じく、置き換えや SIGTERM で 0 で終わった engine は起こさない。
func TestLaunchdPlistRestartsTheEngineOnlyAfterAFailure(t *testing.T) {
	plist, err := launchdPlist("/opt/sshc/bin/sshc", "/Users/someone")
	if err != nil {
		t.Fatal(err)
	}
	restartOnlyAfterFailure := "<key>KeepAlive</key>\n  <dict>\n    <key>SuccessfulExit</key>\n    <false/>\n  </dict>\n"
	if !strings.Contains(plist, restartOnlyAfterFailure) {
		t.Fatalf("plist does not restrict KeepAlive to a failed exit:\n%s", plist)
	}
}

func TestLaunchdServiceFindsAPlistWrittenBeforeKeepAliveWasLimitedToFailures(t *testing.T) {
	manager := testLaunchdServiceManager(t, &fakeLaunchdCommandRunner{})
	if outdated, err := manager.IsDefinitionOutdated(); err != nil || outdated {
		t.Fatalf("absent plist outdated=%v err=%v", outdated, err)
	}
	executable := "/opt/sshc & tools/bin/sshc"
	current, err := launchdPlist(executable, manager.home)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(manager.plistPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manager.plistPath(), []byte(current), 0o600); err != nil {
		t.Fatal(err)
	}
	if outdated, err := manager.IsDefinitionOutdated(); err != nil || outdated {
		t.Fatalf("current plist outdated=%v err=%v", outdated, err)
	}
	restartOnlyAfterFailure := "<key>KeepAlive</key>\n  <dict>\n    <key>SuccessfulExit</key>\n    <false/>\n  </dict>\n"
	alwaysRestart := "<key>KeepAlive</key>\n  <true/>\n"
	older := strings.Replace(current, restartOnlyAfterFailure, alwaysRestart, 1)
	if err := os.WriteFile(manager.plistPath(), []byte(older), 0o600); err != nil {
		t.Fatal(err)
	}
	if outdated, err := manager.IsDefinitionOutdated(); err != nil || !outdated {
		t.Fatalf("older plist outdated=%v err=%v", outdated, err)
	}
	if matches, err := manager.definitionFile().matches(executable); err != nil || matches {
		t.Fatalf("older plist matches=%v err=%v", matches, err)
	}
	if err := outdatedDefinitionError(manager.definitionFile()); !errors.Is(err, errOutdatedServiceDefinition) {
		t.Fatalf("outdated definition error = %v", err)
	}
	manual := []byte("<plist><dict><key>Label</key><string>custom</string></dict></plist>")
	if err := os.WriteFile(manager.plistPath(), manual, 0o600); err != nil {
		t.Fatal(err)
	}
	if outdated, err := manager.IsDefinitionOutdated(); err != nil || outdated {
		t.Fatalf("unmanaged plist outdated=%v err=%v", outdated, err)
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
	runner := &fakeLaunchdCommandRunner{results: []serviceCommandResult{{ExitCode: 0, Output: []byte("pid = 4242\n")}, {ExitCode: 113, Output: []byte("Could not find service")}}}
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
	// print (active), kickstart, print again (still active after the restart).
	runner := &fakeLaunchdCommandRunner{results: []serviceCommandResult{
		{ExitCode: 0, Output: []byte("pid = 4242\n")}, {ExitCode: 0}, {ExitCode: 0, Output: []byte("pid = 4243\n")},
	}}
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
	runner.results = []serviceCommandResult{{ExitCode: 0}}
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

func TestLaunchdServiceKickstartDoesNotReportAnAgentGoneAfterTheCheck(t *testing.T) {
	// print (active), kickstart, print (the agent is gone).
	runner := &fakeLaunchdCommandRunner{results: []serviceCommandResult{
		{ExitCode: 0, Output: []byte("pid = 4242\n")}, {ExitCode: 0}, {ExitCode: 113, Output: []byte("Could not find service")},
	}}
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
	if err != nil || restarted {
		t.Fatalf("restart = %v, %v", restarted, err)
	}
	if got := strings.Join(runner.calls[1], " "); got != "kickstart -k gui/501/"+launchdServiceLabel {
		t.Fatalf("restart call = %q", got)
	}
}

func TestLaunchdReadinessRequiresTheLaunchdPIDAndStatusAPI(t *testing.T) {
	// engineTestServer は testHandoff の秘密で challenge に答える。handoff の PID は 4242 である。
	secret := testHandoff("").Secret
	server := engineTestServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get(handoff.HeaderName) != secret {
			http.Error(writer, "forbidden", http.StatusForbidden)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"owner":"engine","version":"test","protocolVersion":1,"vault":false,"unlocked":false,"sessions":0}`)
	}))
	defer server.Close()
	runner := &fakeLaunchdCommandRunner{results: []serviceCommandResult{{Output: []byte("state = running\n\tpid = 4242\n")}}}
	manager := testLaunchdServiceManager(t, runner)
	writeTestHandoff(t, app.HandoffDir(manager.home), server.URL)
	if err := waitForLaunchdServiceReady(context.Background(), manager); err != nil {
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
