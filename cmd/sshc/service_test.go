package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"io"
	"strings"
	"testing"
)

type recordingServiceLoginItem struct {
	enabled        bool
	registeredErr  error
	enableCalls    int
	disableCalls   int
	enabledProgram string
	enableErr      error
	disableErr     error
}

func (item *recordingServiceLoginItem) Registered() (bool, error) {
	return item.enabled, item.registeredErr
}

func (item *recordingServiceLoginItem) Enable(_ context.Context, program string) error {
	item.enableCalls++
	item.enabledProgram = program
	return item.enableErr
}

func (item *recordingServiceLoginItem) Disable(context.Context) error {
	item.disableCalls++
	return item.disableErr
}

// service は引数が足りなくても多くてもSSH aliasではない。そうでなければ、操作を
// 打ち間違えただけで `ssh -- service` が始まり、保守コマンドのusageが見えない。
func TestServiceIsAlwaysACommandAndNeverAnAlias(t *testing.T) {
	for _, argv := range [][]string{
		{"sshc", "service"},
		{"sshc", "service", "refresh"},
		{"sshc", "service", "disable"},
		{"sshc", "service", "refresh", "extra"},
	} {
		if !serviceInvocation(argv) {
			t.Errorf("serviceInvocation(%q) = false", argv)
		}
		if alias, ok := connectInvocation(argv); ok {
			t.Errorf("connectInvocation(%q) accepted alias %q", argv, alias)
		}
	}
	if serviceInvocation([]string{"sshc", "bastion"}) {
		t.Error("an ordinary alias was read as service maintenance")
	}
}

// オフは既定値であり、installしただけでオンにしてはならない。実行ファイルの解決すら
// しないことを確かめるのは、no-opがEnableの直前で止まるだけの実装を捕まえるためである。
func TestServiceRefreshDoesNotEnableADisabledService(t *testing.T) {
	item := &recordingServiceLoginItem{}
	resolverCalled := false
	var stdout, stderr bytes.Buffer
	code := runService(context.Background(), []string{"refresh"}, item, func() (string, error) {
		resolverCalled = true
		return "/tmp/sshc", nil
	}, &stdout, &stderr)

	if code != 0 || resolverCalled || item.enableCalls != 0 {
		t.Fatalf("code=%d resolverCalled=%v enableCalls=%d", code, resolverCalled, item.enableCalls)
	}
	if !strings.Contains(stdout.String(), "not enabled") || stderr.Len() != 0 {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

// systemdが無いうえにunitも無いLinuxはnil controllerになる。それも既定オフと同じ
// 安全な状態であり、installを失敗させたり起動設定を作ったりしない。
func TestServiceMaintenanceIsANoopWhenThePlatformHasNoController(t *testing.T) {
	for _, action := range []string{"refresh", "disable"} {
		var stdout, stderr bytes.Buffer
		code := runService(context.Background(), []string{action}, nil, func() (string, error) {
			t.Fatal("the executable was resolved for an unsupported, disabled service")
			return "", nil
		}, &stdout, &stderr)
		if code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "not enabled") {
			t.Errorf("%s: code=%d stdout=%q stderr=%q", action, code, stdout.String(), stderr.String())
		}
	}
}

// 「登録なし」と「登録状態を読めない」は違う。後者をno-op成功にすると、uninstallが
// KeepAlive設定の有無を確かめられないまま実行ファイルだけ消せてしまう。
func TestServiceMaintenanceRefusesAnUnknownRegistrationState(t *testing.T) {
	item := &recordingServiceLoginItem{registeredErr: errors.New("permission denied")}
	var stderr bytes.Buffer
	code := runService(context.Background(), []string{"disable"}, item,
		func() (string, error) { return "", nil }, io.Discard, &stderr)
	if code == 0 || item.disableCalls != 0 || !strings.Contains(stderr.String(), "permission denied") {
		t.Fatalf("code=%d disableCalls=%d stderr=%q", code, item.disableCalls, stderr.String())
	}
}

// refreshが受け取れるプログラムは、その保守コマンド自身だけである。argvから任意の
// パスを受け取る実装にすると、Makefile以外から別プログラムを永続化できてしまう。
func TestServiceRefreshRebindsAnEnabledServiceToThisExecutable(t *testing.T) {
	item := &recordingServiceLoginItem{enabled: true}
	var stdout, stderr bytes.Buffer
	code := runService(context.Background(), []string{"refresh"}, item,
		func() (string, error) { return "/Users/tester/.local/bin/sshc", nil },
		&stdout, &stderr)

	if code != 0 || item.enableCalls != 1 || item.enabledProgram != "/Users/tester/.local/bin/sshc" {
		t.Fatalf("code=%d calls=%d program=%q", code, item.enableCalls, item.enabledProgram)
	}
	if !strings.Contains(stdout.String(), "vault is locked") || stderr.Len() != 0 {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestServiceRefreshRefusesAResolvedRelativeExecutable(t *testing.T) {
	item := &recordingServiceLoginItem{enabled: true}
	var stderr bytes.Buffer
	code := runService(context.Background(), []string{"refresh"}, item,
		func() (string, error) { return "bin/sshc", nil }, io.Discard, &stderr)

	if code == 0 || item.enableCalls != 0 || !strings.Contains(stderr.String(), "absolute") {
		t.Fatalf("code=%d calls=%d stderr=%q", code, item.enableCalls, stderr.String())
	}
}

func TestServiceRefreshReportsResolutionAndEnableFailures(t *testing.T) {
	for _, test := range []struct {
		name      string
		item      *recordingServiceLoginItem
		resolve   func() (string, error)
		wantError string
	}{
		{
			name: "resolve",
			item: &recordingServiceLoginItem{enabled: true},
			resolve: func() (string, error) {
				return "", errors.New("cannot resolve executable")
			},
			wantError: "cannot resolve executable",
		},
		{
			name:      "enable",
			item:      &recordingServiceLoginItem{enabled: true, enableErr: errors.New("bootstrap failed")},
			resolve:   func() (string, error) { return "/tmp/sshc", nil },
			wantError: "bootstrap failed",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stderr bytes.Buffer
			code := runService(context.Background(), []string{"refresh"}, test.item,
				test.resolve, io.Discard, &stderr)
			if code == 0 || !strings.Contains(stderr.String(), test.wantError) {
				t.Fatalf("code=%d stderr=%q", code, stderr.String())
			}
		})
	}
}

func TestServiceDisableStopsOnlyAnEnabledService(t *testing.T) {
	item := &recordingServiceLoginItem{enabled: true}
	var stdout, stderr bytes.Buffer
	code := runService(context.Background(), []string{"disable"}, item,
		func() (string, error) { t.Fatal("disable resolved the executable"); return "", nil },
		&stdout, &stderr)

	if code != 0 || item.disableCalls != 1 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "disabled") {
		t.Fatalf("code=%d calls=%d stdout=%q stderr=%q", code, item.disableCalls, stdout.String(), stderr.String())
	}

	item = &recordingServiceLoginItem{enabled: true, disableErr: errors.New("bootout failed")}
	stderr.Reset()
	code = runService(context.Background(), []string{"disable"}, item,
		func() (string, error) { return "", nil }, io.Discard, &stderr)
	if code == 0 || !strings.Contains(stderr.String(), "bootout failed") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestServiceRejectsUnknownOrMalformedActionsWithoutTouchingTheService(t *testing.T) {
	for _, arguments := range [][]string{nil, {"restart"}, {"refresh", "extra"}} {
		item := &recordingServiceLoginItem{enabled: true}
		var stderr bytes.Buffer
		code := runService(context.Background(), arguments, item,
			func() (string, error) { t.Fatal("invalid usage resolved the executable"); return "", nil },
			io.Discard, &stderr)
		if code != 2 || item.enableCalls != 0 || item.disableCalls != 0 ||
			!strings.Contains(stderr.String(), "sshc service refresh|disable") {
			t.Errorf("arguments=%q code=%d enable=%d disable=%d stderr=%q",
				arguments, code, item.enableCalls, item.disableCalls, stderr.String())
		}
	}
}

// mainの入口もactionを先に検証する。そうしないとLinuxでsystemctlやunitの状態が
// 壊れているときだけ、単なる打ち間違いがusageではなく環境エラーになってしまう。
func TestServiceCommandRejectsInvalidUsageBeforeInspectingHomeOrPlatform(t *testing.T) {
	for _, arguments := range [][]string{nil, {"restart"}, {"disable", "extra"}} {
		var stderr bytes.Buffer
		code := runServiceCommand(
			context.Background(), arguments,
			func() (string, error) {
				t.Fatal("invalid usage inspected the home directory")
				return "", nil
			},
			func(string) (serviceLoginItem, error) {
				t.Fatal("invalid usage inspected the platform service")
				return nil, nil
			},
			func() (string, error) {
				t.Fatal("invalid usage resolved the executable")
				return "", nil
			},
			io.Discard, &stderr,
		)
		if code != 2 || !strings.Contains(stderr.String(), serviceUsage) {
			t.Errorf("arguments=%q code=%d stderr=%q", arguments, code, stderr.String())
		}
	}
}

func TestServiceCommandBuildsThePlatformControllerOnlyForAValidAction(t *testing.T) {
	item := &recordingServiceLoginItem{}
	var receivedHome string
	code := runServiceCommand(
		context.Background(), []string{"refresh"},
		func() (string, error) { return "/Users/tester", nil },
		func(home string) (serviceLoginItem, error) {
			receivedHome = home
			return item, nil
		},
		func() (string, error) { return "/tmp/sshc", nil },
		io.Discard, io.Discard,
	)
	if code != 0 || receivedHome != "/Users/tester" {
		t.Fatalf("code=%d home=%q", code, receivedHome)
	}
}

func TestUsageNamesBothServiceMaintenanceActions(t *testing.T) {
	var output bytes.Buffer
	previous := flag.CommandLine.Output()
	flag.CommandLine.SetOutput(&output)
	t.Cleanup(func() { flag.CommandLine.SetOutput(previous) })
	usage(&output)

	for _, command := range []string{"sshc service refresh", "sshc service disable"} {
		if !strings.Contains(output.String(), command) {
			t.Errorf("usage does not contain %q:\n%s", command, output.String())
		}
	}
}
