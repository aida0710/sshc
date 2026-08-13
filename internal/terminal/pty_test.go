//go:build unix

package terminal_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"sshc/internal/terminal"
)

// 本物の PTY を使う検査はこの 1 本だけである。
//
// 上の検査はすべて Process インターフェースの偽物に対して走るので、レジストリの
// ふるまいを確かめるのに実プロセスは要らない。ここで確かめるのは、その偽物が
// 立っている場所に本物を置いても同じ形で動くこと——確保できること、出力が届く
// こと、終了理由が読めること——だけである。
func TestARealPseudoTerminalCarriesTheOutputAndTheExitStatus(t *testing.T) {
	echo := lookProgram(t, "echo")

	registry := &terminal.Registry{
		Start:  terminal.NewStarter(),
		Limits: func() terminal.Limits { return terminal.Limits{MaxSessions: 2, Scrollback: 16 << 10} },
	}
	session, err := registry.Open(terminal.Spec{
		Kind: terminal.KindShell, Title: "echo",
		Size:    terminal.Size{Cols: 80, Rows: 24},
		Command: terminal.Command{Path: echo, Arguments: []string{"embedded-terminal-canary"}},
	})
	if err != nil {
		t.Fatalf("Open() = %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if session.Exit() != nil && strings.Contains(string(session.Snapshot()), "embedded-terminal-canary") {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if got := string(session.Snapshot()); !strings.Contains(got, "embedded-terminal-canary") {
		t.Fatalf("the scrollback never received the output: %q", got)
	}
	info := session.Exit()
	if info == nil {
		t.Fatal("the session never recorded an exit")
	}
	if info.Code != 0 || info.Signal != "" {
		t.Fatalf("exit = %+v, want a clean one", info)
	}
	if info.At.IsZero() {
		t.Fatal("the exit carried no time")
	}
}

// PTY を確保したうえで、そこに置いたプログラムが無ければ開けない。
func TestOpeningAMissingProgramFails(t *testing.T) {
	registry := &terminal.Registry{Start: terminal.NewStarter(), Limits: terminal.DefaultLimits}
	if _, err := registry.Open(terminal.Spec{
		Kind:    terminal.KindShell,
		Command: terminal.Command{Path: "/nonexistent/embedded-terminal"},
	}); err == nil {
		t.Fatal("Open() accepted a program that does not exist")
	}
	if len(registry.Sessions()) != 0 {
		t.Fatal("a failed start left a session behind")
	}
}

// echo の置き場所はディストリビューションで違う。PATH は見ない——このリポジトリの
// 他のどこもそうしていない——ので、決め打ちの場所を順に確かめる。
func lookProgram(t *testing.T, name string) string {
	t.Helper()
	for _, directory := range []string{"/bin", "/usr/bin"} {
		path := directory + "/" + name
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	t.Skipf("%s was not found; this machine cannot run the real pseudo-terminal check", name)
	return ""
}
