package terminal_test

import (
	"context"
	"testing"

	"sshc/internal/terminal"
	"sshc/internal/testwait"
)

func programStatusOf(session *terminal.Session) terminal.ProgramStatusState {
	if status := session.View().ProgramStatus; status != nil {
		return status.State
	}
	return ""
}

func TestTheEngineAnswersAProgramStatusQueryWithNothingFromAReport(t *testing.T) {
	registry, starter := newRegistry(terminal.Limits{MaxSessions: 4, Scrollback: 1 << 12})
	openShell(t, registry)
	process := starter.last()

	process.feed("\x1b]7501;state=working:id=secret\x1b\\\x1b]7501;?\a")
	testwait.Until(t, func() bool { return process.keystrokes() == "\x1b]7501;?\x1b\\" })
}

func TestAShellPromptEndsWhatTheProgramWasDoingButKeepsItsResult(t *testing.T) {
	registry, starter := newRegistry(terminal.Limits{MaxSessions: 4, Scrollback: 1 << 12})
	session := openShell(t, registry)
	process := starter.last()

	process.feed("\x1b]7501;state=done:id=lint\x1b\\\x1b]7501;state=blocked:kind=permission:app=claude\x1b\\")
	testwait.Until(t, func() bool { return programStatusOf(session) == terminal.ProgramStatusBlocked })
	if app := session.View().ProgramStatus.App; app != "claude" {
		t.Fatalf("app = %q, want claude", app)
	}
	process.feed("\x1b]133;A\a$ ")
	testwait.Until(t, func() bool { return programStatusOf(session) == terminal.ProgramStatusDone })

	// A full reset removes every record.
	process.feed("\x1bc")
	testwait.Until(t, func() bool { return programStatusOf(session) == "" })
}

func TestANewShellDropsWhatTheOldProcessWasDoingButKeepsItsResult(t *testing.T) {
	spy := &openSpy{}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}
	spy.at(0).feed("\x1b]7501;state=error:id=deploy\x1b\\\x1b]7501;state=working:id=build\x1b\\\x1b]7501;state=idle\x1b\\")
	testwait.Until(t, func() bool { return programStatusOf(session) == terminal.ProgramStatusError })

	spy.at(0).exit(transportLost)
	testwait.Until(t, func() bool { return spy.count() >= 2 && session.View().State == terminal.StateConnected })
	spy.at(1).feed("\x1b]7501;state=clear:id=deploy\x1b\\")
	// With the failed result cleared by the new shell, nothing of the old process is left.
	testwait.Until(t, func() bool { return programStatusOf(session) == "" })
}
