package terminal_test

import (
	"context"
	"testing"

	"sshc/internal/terminal"
	"sshc/internal/testwait"
)

func TestReopeningsDescribeTheSessionsStillInUseInTheOrderOpened(t *testing.T) {
	registry, starter := newRegistry(terminal.Limits{MaxSessions: 4, Scrollback: 1 << 10})
	open := func(profile, workspace string) *terminal.Session {
		t.Helper()
		session, err := registry.Open(context.Background(), terminal.Spec{
			Kind: terminal.KindShell, ShellProfile: profile, Title: "zsh", Workspace: workspace,
			Size: terminal.Size{Cols: 80, Rows: 24}, Command: terminal.Command{Path: "/bin/zsh"},
		})
		if err != nil {
			t.Fatalf("Open() = %v", err)
		}
		return session
	}
	first := open("zsh", "default")
	exited := open("bash", "default")
	closed := open("bash", "default")
	open("fish", "work")

	starter.processes[0].feed("\x1b]7;file://edge/home/aida/src\a$ ")
	if err := registry.Rename(first.ID(), "build"); err != nil {
		t.Fatal(err)
	}
	starter.processes[1].exit(terminal.ExitInfo{Code: 0})
	testwait.Until(t, func() bool { view := exited.View(); return view.Exited != nil })
	if err := registry.Close(closed.ID()); err != nil {
		t.Fatal(err)
	}
	testwait.Until(t, func() bool { return first.WorkingDirectory() != "" })

	got := registry.Reopenings()
	want := []terminal.Reopening{
		{Kind: terminal.KindShell, ShellProfile: "zsh", Workspace: "default", PinnedTitle: "build", WorkingDirectory: "/home/aida/src"},
		{Kind: terminal.KindShell, ShellProfile: "fish", Workspace: "work"},
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("Reopenings() = %+v, want %+v", got, want)
	}
}
