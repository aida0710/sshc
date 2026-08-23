//go:build unix

package terminal_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"sshc/internal/terminal"
)

func TestARealPseudoTerminalCarriesTheOutputAndTheExitStatus(t *testing.T) {
	echo := lookProgram(t, "echo")

	registry := &terminal.Registry{
		Start:  terminal.NewStarter(),
		Limits: func() terminal.Limits { return terminal.Limits{MaxSessions: 2, Scrollback: 16 << 10} },
	}
	session, err := registry.Open(context.Background(), terminal.Spec{
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

func TestOpeningAMissingProgramFails(t *testing.T) {
	registry := &terminal.Registry{Start: terminal.NewStarter(), Limits: terminal.DefaultLimits}
	if _, err := registry.Open(context.Background(), terminal.Spec{
		Kind:    terminal.KindShell,
		Command: terminal.Command{Path: "/nonexistent/embedded-terminal"},
	}); err == nil {
		t.Fatal("Open() accepted a program that does not exist")
	}
	if len(registry.Sessions()) != 0 {
		t.Fatal("a failed start left a session behind")
	}
}

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
