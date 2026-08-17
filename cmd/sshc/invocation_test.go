package main

import (
	"strings"
	"testing"
)

// **接続先とコマンドは別の引数である。** ひとつの語に兼ねさせると、どこまでが
// 接続先でどこからがコマンドかを推測することになる。
func TestRunTakesAnAliasAndACommand(t *testing.T) {
	for _, argv := range [][]string{
		{"sshc", "run"},
		{"sshc", "run", "win"},
	} {
		if _, err := parseInvocation(argv); err == nil {
			t.Errorf("%v was accepted; run needs an alias and a command", argv)
		}
	}

	called, err := parseInvocation([]string{"sshc", "run", "win", "go", "test", "./..."})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if called.Kind != invocationRun {
		t.Fatalf("kind = %v, want invocationRun", called.Kind)
	}
	if got := strings.Join(called.Args, "|"); got != "win|go|test|./..." {
		t.Errorf("args = %q", got)
	}
}

// **語は空白ひとつで繋ぐ。** OpenSSH の `ssh host cmd args` と同じで、引用の
// 規則は相手のシェルのものである。こちらで包み直せば、どちらかのシェルで壊れる。
func TestTheRemoteCommandIsJoinedWithoutRequoting(t *testing.T) {
	got := remoteCommand([]string{"go", "test", "./...", "-run", "'Test[A-Z]'"})
	if want := `go test ./... -run 'Test[A-Z]'`; got != want {
		t.Errorf("remoteCommand = %q, want %q", got, want)
	}
}

func TestParseInvocationSeparatesOwnersFromDesktopActivation(t *testing.T) {
	tests := []struct {
		argv []string
		kind invocationKind
		args []string
	}{
		{[]string{"sshc"}, invocationDesktop, nil},
		{[]string{"sshc", "engine"}, invocationEngine, nil},
		{[]string{"sshc", "headless"}, invocationHeadless, nil},
		{[]string{"sshc", "server-a"}, invocationConnect, []string{"server-a"}},
		{[]string{"sshc", "connect"}, invocationChoose, nil},
		{[]string{"sshc", "connect", "prod"}, invocationChoose, []string{"prod"}},
		{[]string{"sshc", "list"}, invocationList, nil},
		{[]string{"sshc", "open"}, invocationOpen, nil},
		{[]string{"sshc", "status"}, invocationStatus, nil},
		{[]string{"sshc", "vault", "status"}, invocationVault, []string{"status"}},
		{[]string{"sshc", "vault", "create"}, invocationVault, []string{"create"}},
		{[]string{"sshc", "vault", "unlock"}, invocationVault, []string{"unlock"}},
		{[]string{"sshc", "vault", "lock"}, invocationVault, []string{"lock"}},
		{[]string{"sshc", "vault", "change-password"}, invocationVault, []string{"change-password"}},
		{[]string{"sshc", "help"}, invocationHelp, nil},
		{[]string{"sshc", "-h"}, invocationHelp, nil},
		{[]string{"sshc", "--help"}, invocationHelp, nil},
	}
	for _, test := range tests {
		got, err := parseInvocation(test.argv)
		if err != nil || got.Kind != test.kind || !sameStrings(got.Args, test.args) {
			t.Fatalf("parseInvocation(%q) = %#v, %v; want kind %v and args %q", test.argv, got, err, test.kind, test.args)
		}
	}
}

func TestUsageNamesEveryVaultAction(t *testing.T) {
	var output strings.Builder
	usage(&output)
	for _, action := range []string{"status", "create", "unlock", "lock", "change-password"} {
		if !strings.Contains(output.String(), "sshc vault "+action) {
			t.Errorf("usage does not name vault action %q:\n%s", action, output.String())
		}
	}
}

func TestParseInvocationRejectsArgumentsThatDoNotNameACommand(t *testing.T) {
	tests := [][]string{
		{"sshc", "--own-engine"},
		{"sshc", "engine", "extra"},
		{"sshc", "vault", "rotate"},
		{"sshc", "server-a", "extra"},
	}
	for _, argv := range tests {
		if got, err := parseInvocation(argv); err == nil || got.Kind != invocationInvalid {
			t.Errorf("parseInvocation(%q) = %#v, %v; want usage error", argv, got, err)
		}
	}
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
