package main

import (
	"bytes"
	"runtime"
	"strings"
	"testing"
	"time"
)

// 非対話SSHは mode flag と delimiter を要求する。これを緩めると接続optionと
// remote commandの境界が、引数の内容によって変わってしまう。
func TestSSHNonInteractiveTakesAnAliasDelimiterAndCommand(t *testing.T) {
	for _, argv := range [][]string{
		{"sshc", "ssh", "win", "--non-interactive"},
		{"sshc", "ssh", "win", "--non-interactive", "--"},
		{"sshc", "ssh", "--non-interactive", "--", "hostname"},
	} {
		if _, err := parseInvocation(argv); err == nil {
			t.Errorf("%v was accepted; non-interactive SSH needs an alias, --, and a command", argv)
		}
	}

	called, err := parseInvocation([]string{"sshc", "ssh", "win", "--non-interactive", "--", "go", "test", "./..."})
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
		{[]string{"sshc", "ssh"}, invocationChoose, nil},
		{[]string{"sshc", "ssh", "headless"}, invocationConnect, []string{"headless"}},
		{[]string{"sshc", "ssh", "server-a"}, invocationConnect, []string{"server-a"}},
		{[]string{"sshc", "ssh", "--list"}, invocationList, nil},
		{[]string{"sshc", "ssh", "--help"}, invocationHelp, nil},
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

func TestParseInfoAndSyncInvocations(t *testing.T) {
	tests := []struct {
		argv    []string
		kind    invocationKind
		alias   string
		action  syncAction
		force   bool
		asJSON  bool
		enabled bool
	}{
		{argv: []string{"sshc", "info", "edge"}, kind: invocationInfo, alias: "edge"},
		{argv: []string{"sshc", "info", "edge", "--json"}, kind: invocationInfo, alias: "edge", asJSON: true},
		{argv: []string{"sshc", "sync"}, kind: invocationSync, action: syncStatus},
		{argv: []string{"sshc", "sync", "--json"}, kind: invocationSync, action: syncStatus, asJSON: true},
		{argv: []string{"sshc", "sync", "setup"}, kind: invocationSync, action: syncSetup},
		{argv: []string{"sshc", "sync", "push", "--force", "--json"}, kind: invocationSync, action: syncPush, force: true, asJSON: true},
		{argv: []string{"sshc", "sync", "pull", "--force"}, kind: invocationSync, action: syncPull, force: true},
		{argv: []string{"sshc", "sync", "now", "--json"}, kind: invocationSync, action: syncNow, asJSON: true},
		{argv: []string{"sshc", "sync", "auto", "on"}, kind: invocationSync, action: syncAuto, enabled: true},
		{argv: []string{"sshc", "sync", "auto", "off", "--json"}, kind: invocationSync, action: syncAuto, asJSON: true},
	}
	for _, test := range tests {
		t.Run(strings.Join(test.argv[1:], "_"), func(t *testing.T) {
			got, err := parseInvocation(test.argv)
			if err != nil {
				t.Fatalf("parseInvocation(%q): %v", test.argv, err)
			}
			if got.Kind != test.kind || got.JSON != test.asJSON {
				t.Fatalf("parseInvocation(%q) = %#v", test.argv, got)
			}
			if test.alias != "" && (len(got.Args) != 1 || got.Args[0] != test.alias) {
				t.Fatalf("parseInvocation(%q) args = %q, want %q", test.argv, got.Args, test.alias)
			}
			if test.kind == invocationSync {
				if got.Sync == nil || got.Sync.Action != test.action || got.Sync.Force != test.force ||
					got.Sync.JSON != test.asJSON || got.Sync.Enabled != test.enabled {
					t.Fatalf("parseInvocation(%q) sync = %#v", test.argv, got.Sync)
				}
			}
		})
	}
}

func TestParseTerminalInvocations(t *testing.T) {
	const id = "01234567"
	tests := []struct {
		argv   []string
		action terminalAction
		check  func(*testing.T, terminalInvocation)
	}{
		{[]string{"sshc", "terminal", "list", "--json"}, terminalList, nil},
		{[]string{"sshc", "terminal", "show", id}, terminalShow, nil},
		{[]string{"sshc", "terminal", "read", id, "--cursor", "9", "--limit", "1024", "--json"}, terminalRead,
			func(t *testing.T, got terminalInvocation) {
				if got.Cursor != 9 || got.Limit != 1024 || !got.JSON {
					t.Fatalf("read = %#v", got)
				}
			}},
		{[]string{"sshc", "terminal", "send", id, "--text", "uptime", "--no-enter"}, terminalSend,
			func(t *testing.T, got terminalInvocation) {
				if got.Text != "uptime" || got.Submit {
					t.Fatalf("send = %#v", got)
				}
			}},
		{[]string{"sshc", "terminal", "wait", id, "--for", "agent-ready", "--timeout", "30s"}, terminalWait,
			func(t *testing.T, got terminalInvocation) {
				if got.WaitFor != "agent-ready" || got.Timeout != 30*time.Second {
					t.Fatalf("wait = %#v", got)
				}
			}},
		{[]string{"sshc", "terminal", "create", "shell"}, terminalCreate, nil},
		{[]string{"sshc", "terminal", "create", "ssh", "bastion", "--json"}, terminalCreate,
			func(t *testing.T, got terminalInvocation) {
				if got.Kind != "ssh" || got.Alias != "bastion" || !got.JSON {
					t.Fatalf("create = %#v", got)
				}
			}},
		{[]string{"sshc", "terminal", "rename", id, "deploy"}, terminalRename, nil},
		{[]string{"sshc", "terminal", "close", id}, terminalClose, nil},
	}
	for _, test := range tests {
		t.Run(strings.Join(test.argv[2:], "_"), func(t *testing.T) {
			called, err := parseInvocation(test.argv)
			if err != nil || called.Kind != invocationTerminal || called.Terminal == nil || called.Terminal.Action != test.action {
				t.Fatalf("parseInvocation(%q) = %#v, %v", test.argv, called, err)
			}
			if test.check != nil {
				test.check(t, *called.Terminal)
			}
		})
	}
}

func TestTerminalInvocationRejectsUnstableSelectorsAndHeuristicWaits(t *testing.T) {
	for _, argv := range [][]string{
		{"sshc", "terminal"},
		{"sshc", "terminal", "show", "title"},
		{"sshc", "terminal", "show", "0123"},
		{"sshc", "terminal", "send", "01234567"},
		{"sshc", "terminal", "send", "01234567", "--text", ""},
		{"sshc", "terminal", "wait", "01234567", "--for", "prompt-visible"},
		{"sshc", "terminal", "wait", "01234567", "--for", "connected", "--timeout", "0s"},
		{"sshc", "terminal", "read", "01234567", "--limit", "65537"},
		{"sshc", "terminal", "create", "ssh"},
	} {
		if got, err := parseInvocation(argv); err == nil || got.Kind != invocationInvalid {
			t.Errorf("parseInvocation(%q) = %#v, %v; want usage error", argv, got, err)
		}
	}
}

func TestInfoAndSyncRejectAmbiguousArguments(t *testing.T) {
	for _, argv := range [][]string{
		{"sshc", "info"},
		{"sshc", "info", "--json", "edge"},
		{"sshc", "info", "edge", "--json", "--json"},
		{"sshc", "info", "edge", "extra"},
		{"sshc", "sync", "setup", "--json"},
		{"sshc", "sync", "setup", "--force"},
		{"sshc", "sync", "push", "--force", "--force"},
		{"sshc", "sync", "now", "--force"},
		{"sshc", "sync", "auto"},
		{"sshc", "sync", "auto", "maybe"},
		{"sshc", "sync", "auto", "on", "--force"},
		{"sshc", "sync", "unknown"},
		{"sshc", "sync", "pull", "extra"},
	} {
		if got, err := parseInvocation(argv); err == nil || got.Kind != invocationInvalid {
			t.Errorf("parseInvocation(%q) = %#v, %v; want usage error", argv, got, err)
		}
	}
}

func TestUsageNamesInfoAndSync(t *testing.T) {
	var output strings.Builder
	usage(&output)
	text := output.String()
	for _, want := range []string{
		"sshc info <alias> [--json]",
		"sshc sync [--json]",
		"sshc sync setup",
		"sshc sync push [--force] [--json]",
		"sshc sync pull [--force] [--json]",
		"sshc sync now [--json]",
		"sshc sync auto on|off [--json]",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("usage does not contain %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "sshc connect") {
		t.Errorf("usage restored the retired connect command:\n%s", text)
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

// help が旧構文を残すと、parserが意図的に拒否する入口へ利用者を誘導する。
func TestUsageDocumentsOnlyExplicitTransportCommands(t *testing.T) {
	var output strings.Builder
	usage(&output)
	text := output.String()
	for _, want := range []string{
		"sshc ssh [<alias>]",
		"sshc ssh <alias> --non-interactive -- <command>",
		"sshc serial [--json]",
		"sshc serial <device>",
		"sshc telnet <host>[:port]",
		"--non-interactive",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("usage does not contain %q:\n%s", want, text)
		}
	}
	for _, obsolete := range []string{
		"sshc <alias>",
		"sshc run",
		"sshc connect",
		"sshc list",
		"sshc serial list",
	} {
		if strings.Contains(text, obsolete) {
			t.Errorf("usage still contains obsolete syntax %q:\n%s", obsolete, text)
		}
	}
}

func TestParseInvocationRejectsArgumentsThatDoNotNameACommand(t *testing.T) {
	tests := [][]string{
		{"sshc", "--own-engine"},
		{"sshc", "engine", "extra"},
		{"sshc", "vault", "rotate"},
		{"sshc", "server-a"},
		{"sshc", "server-a", "extra"},
		{"sshc", "connect"},
		{"sshc", "list"},
		{"sshc", "run", "server-a", "hostname"},
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

func TestUpdateIsReservedAndTakesNoArguments(t *testing.T) {
	called, err := parseInvocation([]string{"sshc", "update"})
	if err != nil {
		t.Fatalf("parse update = %v", err)
	}
	if called.Kind != invocationUpdate {
		t.Fatalf("update kind = %v, want invocationUpdate", called.Kind)
	}
	if _, err := parseInvocation([]string{"sshc", "update", "later"}); err == nil {
		t.Fatal("update accepted an argument")
	}

	var out bytes.Buffer
	usage(&out)
	if !strings.Contains(out.String(), "sshc update") {
		t.Error("usage does not mention the update command")
	}
}
