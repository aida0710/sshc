package terminal_test

import (
	"context"
	"sync"
	"testing"

	"sshc/internal/terminal"
	"sshc/internal/testwait"
)

// blockedProgramLog keeps what Registry.ProgramBlocked heard.
type blockedProgramLog struct {
	mutex   sync.Mutex
	changes []terminal.BlockedProgramChange
}

func (l *blockedProgramLog) add(change terminal.BlockedProgramChange) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.changes = append(l.changes, change)
}

func (l *blockedProgramLog) heard() []terminal.BlockedProgramChange {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return append([]terminal.BlockedProgramChange(nil), l.changes...)
}

func (l *blockedProgramLog) waitForCount(t *testing.T, want int) []terminal.BlockedProgramChange {
	t.Helper()
	testwait.Until(t, func() bool { return len(l.heard()) >= want })
	return l.heard()
}

func openWatchedShell(t *testing.T) (*terminal.Session, *fakeProcess, *blockedProgramLog) {
	t.Helper()
	registry, starter := newRegistry(terminal.Limits{MaxSessions: 4, Scrollback: 1 << 12})
	log := &blockedProgramLog{}
	registry.ProgramBlocked = log.add
	session := openShell(t, registry)
	return session, starter.last(), log
}

func TestABlockedProgramIsHeardOnceWithWhatItWaitsFor(t *testing.T) {
	session, process, log := openWatchedShell(t)

	process.feed("\x1b]7501;state=working\x1b\\\x1b]7501;state=blocked:kind=permission:msg=YXBwcm92ZQ\x1b\\")
	changes := log.waitForCount(t, 1)
	if changes[0].SessionID != session.ID() || changes[0].SessionTitle != "zsh" {
		t.Fatalf("change = %+v, want session %s named zsh", changes[0], session.ID())
	}
	status := changes[0].Status
	if status == nil || status.Kind != terminal.ProgramStatusPermission || status.Message != "approve" {
		t.Fatalf("status = %+v, want permission with the message", status)
	}

	// Reporting again while blocked, with other words or as another record, says nothing new: the next change heard is
	// the end of the wait.
	process.feed("\x1b]7501;state=blocked:kind=permission:msg=YWdhaW4\x1b\\\x1b]7501;state=blocked:kind=question:id=sub\x1b\\")
	process.feed("\x1b]7501;state=working\x1b\\\x1b]7501;state=working:id=sub\x1b\\")
	if changes = log.waitForCount(t, 2); changes[1].Status != nil {
		t.Fatalf("changes = %+v, want the second to say nothing waits", changes)
	}
}

func TestAProgramIsHeardToStopWaitingOnceNoRecordIsBlocked(t *testing.T) {
	_, process, log := openWatchedShell(t)

	process.feed("\x1b]7501;state=blocked:id=one\x1b\\\x1b]7501;state=blocked:id=two\x1b\\")
	log.waitForCount(t, 1)
	process.feed("\x1b]7501;state=working:id=one\x1b\\\x1b]7501;state=idle:id=two\x1b\\")
	changes := log.waitForCount(t, 2)
	if len(changes) != 2 || changes[1].Status != nil {
		t.Fatalf("changes = %+v, want one that says nothing waits", changes)
	}
}

func TestAProgramWaitingAgainIsHeardAgain(t *testing.T) {
	_, process, log := openWatchedShell(t)

	process.feed("\x1b]7501;state=blocked\x1b\\\x1b]7501;state=working\x1b\\\x1b]7501;state=blocked\x1b\\")
	changes := log.waitForCount(t, 3)
	if changes[0].Status == nil || changes[1].Status != nil || changes[2].Status == nil {
		t.Fatalf("changes = %+v, want blocked, not blocked, blocked", changes)
	}
}

func TestAShellPromptOrAResetEndsTheWait(t *testing.T) {
	for name, ending := range map[string]string{"prompt": "\x1b]133;A\a$ ", "reset": "\x1bc"} {
		t.Run(name, func(t *testing.T) {
			_, process, log := openWatchedShell(t)

			process.feed("\x1b]7501;state=blocked\x1b\\")
			log.waitForCount(t, 1)
			process.feed(ending)
			if changes := log.waitForCount(t, 2); changes[1].Status != nil {
				t.Fatalf("changes = %+v, want one that says nothing waits", changes)
			}
		})
	}
}

func TestASessionThatEndsNoLongerWaits(t *testing.T) {
	_, process, log := openWatchedShell(t)

	process.feed("\x1b]7501;state=blocked\x1b\\")
	log.waitForCount(t, 1)
	process.exit(terminal.ExitInfo{Code: 0})
	if changes := log.waitForCount(t, 2); changes[1].Status != nil {
		t.Fatalf("changes = %+v, want one that says nothing waits", changes)
	}
}

func TestASessionThatNeverWaitedSaysNothingWhenItEnds(t *testing.T) {
	session, process, log := openWatchedShell(t)

	process.feed("\x1b]7501;state=working\x1b\\")
	testwait.Until(t, func() bool { return programStatusOf(session) == terminal.ProgramStatusWorking })
	process.exit(terminal.ExitInfo{Code: 0})
	testwait.Until(t, func() bool { return !session.Live() })
	if heard := log.heard(); len(heard) != 0 {
		t.Fatalf("heard %+v, want nothing", heard)
	}
}

// A dropped connection does not tell whether the program still waits (under tmux it does), so the person keeps the
// notification until the program, reached again, says otherwise.
func TestAReconnectLeavesTheWaitUntilTheNewProcessReports(t *testing.T) {
	spy := &openSpy{}
	registry, _ := newFastRegistry()
	log := &blockedProgramLog{}
	registry.ProgramBlocked = log.add
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}
	spy.at(0).feed("\x1b]7501;state=blocked\x1b\\")
	if changes := log.waitForCount(t, 1); changes[0].SessionAlias != "gateway" {
		t.Fatalf("change = %+v, want the connection it runs on", changes[0])
	}

	spy.at(0).exit(transportLost)
	testwait.Until(t, func() bool { return spy.count() >= 2 && session.View().State == terminal.StateConnected })
	if heard := log.heard(); len(heard) != 1 {
		t.Fatalf("a reconnect was heard: %+v", heard)
	}

	spy.at(1).feed("\x1b]7501;state=working\x1b\\")
	if changes := log.waitForCount(t, 2); changes[1].Status != nil {
		t.Fatalf("changes = %+v, want one that says nothing waits", changes)
	}
}
