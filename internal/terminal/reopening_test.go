package terminal_test

import (
	"context"
	"testing"
	"time"

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

func TestAnSSHSessionThatLostItsConnectionIsStillReopened(t *testing.T) {
	spy := &openSpy{}
	registry, _ := newRegistry(terminal.DefaultLimits())
	registry.ReconnectDelay = func(int) time.Duration { return time.Hour }
	open := func(alias string) *terminal.Session {
		t.Helper()
		session, err := registry.Open(context.Background(), terminal.Spec{
			Kind: terminal.KindSSH, Alias: alias, Title: alias, Workspace: "default", Open: spy.open,
		})
		if err != nil {
			t.Fatal(err)
		}
		return session
	}
	waiting := open("waiting")
	stopped := open("stopped")
	finished := open("finished")
	spy.at(0).exit(transportLost)
	spy.at(1).exit(transportLost)
	spy.at(2).exit(terminal.ExitInfo{Code: 0})
	testwait.Until(t, func() bool { return waiting.View().State == terminal.StateReconnecting })
	testwait.Until(t, func() bool { return stopped.View().State == terminal.StateReconnecting })
	if err := registry.StopReconnecting(context.Background(), stopped.ID()); err != nil {
		t.Fatal(err)
	}
	testwait.Until(t, func() bool { return stopped.View().State == terminal.StateExited })
	testwait.Until(t, func() bool { return finished.View().State == terminal.StateExited })

	var aliases []string
	for _, reopening := range registry.Reopenings() {
		aliases = append(aliases, reopening.Alias)
	}
	if len(aliases) != 2 || aliases[0] != "waiting" || aliases[1] != "stopped" {
		t.Fatalf("reopened %q, want the waiting and the stopped session but not the finished one", aliases)
	}
	for _, session := range []*terminal.Session{waiting, stopped, finished} {
		_ = registry.Close(session.ID())
	}
}
