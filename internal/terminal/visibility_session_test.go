package terminal_test

import (
	"context"
	"strings"
	"testing"

	"sshc/internal/terminal"
	"sshc/internal/testwait"
)

const (
	reportVisible = "\x1b[?999;1n"
	reportHidden  = "\x1b[?999;2n"
)

func TestVisibilityReportsFollowTheBrowsersShowingTheSession(t *testing.T) {
	registry, starter := newRegistry(terminal.Limits{MaxSessions: 4, Scrollback: 1 << 12})
	session := openShell(t, registry)
	process := starter.last()
	answered := func(want string) {
		t.Helper()
		testwait.Until(t, func() bool { return process.keystrokes() == want })
	}

	// Nobody is attached, so nobody can see it.
	process.feed("\x1b[?2033h")
	answered(reportHidden)

	_, first := attach(t, session)
	answered(reportHidden + reportVisible)
	_, second := attach(t, session)
	session.SetStreamVisible(first, false)
	// The second browser still shows it; nothing changed, nothing is sent.
	process.feed("\x1b[?2033$p")
	answered(reportHidden + reportVisible + "\x1b[?2033;1$y")
	session.SetStreamVisible(second, false)
	answered(reportHidden + reportVisible + "\x1b[?2033;1$y" + reportHidden)
	session.Detach(second)
	session.SetStreamVisible(first, true)
	answered(reportHidden + reportVisible + "\x1b[?2033;1$y" + reportHidden + reportVisible)
}

func TestAQueryAnswersOnceAndTurnsNoReportsOn(t *testing.T) {
	registry, starter := newRegistry(terminal.Limits{MaxSessions: 4, Scrollback: 1 << 12})
	session := openShell(t, registry)
	process := starter.last()
	_, stream := attach(t, session)

	process.feed("\x1b[?998n")
	testwait.Until(t, func() bool { return process.keystrokes() == reportVisible })
	process.feed("\x1b[?2033$p")
	testwait.Until(t, func() bool { return process.keystrokes() == reportVisible+"\x1b[?2033;2$y" })

	// Turned off, a change is not reported. The query after it shows the reports kept quiet.
	session.SetStreamVisible(stream, false)
	process.feed("\x1b[?998n")
	testwait.Until(t, func() bool { return strings.HasSuffix(process.keystrokes(), reportHidden) })
	if got := process.keystrokes(); got != reportVisible+"\x1b[?2033;2$y"+reportHidden {
		t.Fatalf("answers = %q, want no report of the change itself", got)
	}
}

// A program that turned reports on and was cut off must not have them typed
// into the next shell, which never asked for them.
func TestAProgramsVisibilityReportsEndWithItsShell(t *testing.T) {
	spy := &openSpy{}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, stream := attach(t, session)
	spy.at(0).feed("\x1b[?2033h")
	testwait.Until(t, func() bool { return spy.at(0).keystrokes() == reportVisible })

	spy.at(0).exit(transportLost)
	testwait.Until(t, func() bool { return spy.count() >= 2 && session.View().State == terminal.StateConnected })
	session.SetStreamVisible(stream, false)
	// Asking whether reports are on shows the new shell is answered at all, and that the change before it
	// was not reported.
	spy.at(1).feed("\x1b[?2033$p")
	testwait.Until(t, func() bool { return strings.HasSuffix(spy.at(1).keystrokes(), "$y") })
	if got := spy.at(1).keystrokes(); got != "\x1b[?2033;2$y" {
		t.Fatalf("the new shell got %q, want only that reports are off", got)
	}
}
