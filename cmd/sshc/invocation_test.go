package main

import (
	"runtime"
	"strings"
	"testing"
)

// 接続先とコマンドは別の引数である。ひとつの語に兼ねさせると、どこまでが
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

// 語は空白ひとつで繋ぐ。OpenSSH の `ssh host cmd args` と同じで、引用の
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
		// headless はもう予約語ではない。engine を持つ道が 1 つになったので、
		// この表記は他と同じただの接続先として読まれる。
		{[]string{"sshc", "headless"}, invocationConnect, []string{"headless"}},
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

// 版を訊く道は、打たれる形すべてで通らなければならない。
//
// `sshc version` が正式だが、`--version` は誰もが最初に打つ形である。受けないと
// usage と終了コード 2 になり、入れた直後の一行目が失敗する。実際
// docs/release-install.md は `./sshc --version` を案内していた。
func TestEveryWayOfAskingForTheVersionIsAccepted(t *testing.T) {
	for _, word := range []string{"version", "--version", "-v"} {
		called, err := parseInvocation([]string{"sshc", word})
		if err != nil {
			t.Fatalf("%s: %v", word, err)
		}
		if called.Kind != invocationVersion {
			t.Errorf("%s: kind = %v, want invocationVersion", word, called.Kind)
		}
	}
}

// 版は引数を取らない。取ると alias と見分けが付かなくなる。
func TestAskingForTheVersionTakesNoArguments(t *testing.T) {
	if _, err := parseInvocation([]string{"sshc", "version", "extra"}); err == nil {
		t.Fatal("version accepted an argument")
	}
}

// 入っているものを言うときは、どの機械のものかも言う。入れ方が増えたので、
// 「入ったが動かない」の相談で最初に要るのは版よりもそちらである。
func TestTheVersionLineNamesTheBuildTarget(t *testing.T) {
	var out strings.Builder
	printVersion(&out)
	line := out.String()
	for _, want := range []string{"sshc ", version, runtime.GOOS, runtime.GOARCH} {
		if !strings.Contains(line, want) {
			t.Errorf("version line %q does not mention %q", line, want)
		}
	}
}

// usage は、打てるものを全部並べる約束である。
func TestUsageNamesTheVersionCommand(t *testing.T) {
	var out strings.Builder
	usage(&out)
	if !strings.Contains(out.String(), "sshc version") {
		t.Error("usage does not mention the version command")
	}
}
