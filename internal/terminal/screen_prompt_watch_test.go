package terminal_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"sshc/internal/terminal"
	"sshc/internal/testwait"
)

// tmuxOutput is what a tmux client wrote to its terminal while real Claude Code 2.1.318 asked, in a window of the size
// the file name says, and then what it wrote once the person had answered.
func tmuxOutput(t *testing.T, name string) (asking, answered string) {
	t.Helper()
	read := func(file string) string {
		output, err := os.ReadFile(filepath.Join("testdata", "claude_code_prompts", "tmux-output", file))
		if err != nil {
			t.Fatal(err)
		}
		return string(output)
	}
	return read(name + ".raw"), read(name + ".answered.raw")
}

func TestAQuestionUnderTmuxIsHeardAndEndsWhenAnswered(t *testing.T) {
	asking, answered := tmuxOutput(t, "question-80x24")
	session, process, log := openWatchedShell(t)

	process.feed(asking)
	changes := log.waitForCount(t, 1)
	if status := changes[0].Status; status == nil || status.Kind != terminal.ProgramStatusQuestion {
		t.Fatalf("status = %+v, want a question", status)
	}
	if status := session.View().ProgramStatus; status == nil || status.State != terminal.ProgramStatusBlocked {
		t.Fatalf("session list shows %+v, want blocked", status)
	}

	process.feed(answered)
	changes = log.waitForCount(t, 2)
	if changes[1].Status != nil {
		t.Fatalf("status = %+v, want none once answered", changes[1].Status)
	}
	if status := session.View().ProgramStatus; status != nil {
		t.Fatalf("session list shows %+v, want nothing", status)
	}
}

func TestAPermissionInOnePaneOfASplitWindowUnderTmuxIsHeard(t *testing.T) {
	asking, answered := tmuxOutput(t, "permission-split-right-40x30")
	session, process, log := openWatchedShell(t)
	if err := session.Resize(terminal.Size{Cols: 40, Rows: 30}); err != nil {
		t.Fatal(err)
	}

	process.feed(asking)
	if status := log.waitForCount(t, 1)[0].Status; status == nil || status.Kind != terminal.ProgramStatusPermission {
		t.Fatalf("status = %+v, want a permission", status)
	}
	process.feed(answered)
	if changes := log.waitForCount(t, 2); changes[1].Status != nil {
		t.Fatalf("status = %+v, want none once answered", changes[1].Status)
	}
}

func TestASizeChangedOnTheAlternateScreenIsFollowed(t *testing.T) {
	asking, _ := tmuxOutput(t, "permission-split-right-40x30")
	session, process, log := openWatchedShell(t)
	enter := "\x1b[?1049h"
	process.feed(enter)
	// The output reaches the emulator before the buffer, so the size change comes after that.
	testwait.Until(t, func() bool { return session.CanAttachFrom(uint64(len(enter))) })

	if err := session.Resize(terminal.Size{Cols: 40, Rows: 30}); err != nil {
		t.Fatal(err)
	}
	process.feed(asking)
	if status := log.waitForCount(t, 1)[0].Status; status == nil || status.Kind != terminal.ProgramStatusPermission {
		t.Fatalf("status = %+v, want a permission", status)
	}
}

func TestDetachingFromTmuxEndsTheWaitAtOnce(t *testing.T) {
	asking, _ := tmuxOutput(t, "question-80x24")
	_, process, log := openWatchedShell(t)

	process.feed(asking)
	log.waitForCount(t, 1)
	process.feed("\x1b[?1049l[detached (from session lab)]\r\n$ ")
	if changes := log.waitForCount(t, 2); changes[1].Status != nil {
		t.Fatalf("status = %+v, want none after leaving tmux", changes[1].Status)
	}
}

func TestReattachingToTmuxKeepsTheWaitWithoutNotifyingAgain(t *testing.T) {
	asking, answered := tmuxOutput(t, "question-80x24")
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

	spy.at(0).feed(asking)
	log.waitForCount(t, 1)
	spy.at(0).exit(transportLost)
	testwait.Until(t, func() bool { return spy.count() >= 2 && session.View().State == terminal.StateConnected })

	// tmux draws the same question again for the new client.
	spy.at(1).feed(asking)
	testwait.Until(t, func() bool { return programStatusOf(session) == terminal.ProgramStatusBlocked })
	if changes := log.heard(); len(changes) != 1 {
		t.Fatalf("heard %+v, want the wait told once", changes)
	}

	spy.at(1).feed(answered)
	if changes := log.waitForCount(t, 2); changes[1].Status != nil {
		t.Fatalf("status = %+v, want none once answered", changes[1].Status)
	}
}

func TestReattachingToTmuxWithTheQuestionGoneEndsTheWait(t *testing.T) {
	asking, _ := tmuxOutput(t, "question-80x24")
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

	spy.at(0).feed(asking)
	log.waitForCount(t, 1)
	spy.at(0).exit(transportLost)
	testwait.Until(t, func() bool { return spy.count() >= 2 && session.View().State == terminal.StateConnected })

	spy.at(1).feed("\x1b[?1049h\x1b[H$ ")
	if changes := log.waitForCount(t, 2); changes[1].Status != nil {
		t.Fatalf("status = %+v, want none with the question gone", changes[1].Status)
	}
}

func TestTheAlternateScreenEnteredAcrossChunksIsFollowed(t *testing.T) {
	asking, _ := tmuxOutput(t, "question-80x24")
	_, process, log := openWatchedShell(t)
	cut := bytes.Index([]byte(asking), []byte("\x1b[?1049h")) + len("\x1b[?10")
	if cut < len("\x1b[?10") {
		t.Fatal("the recorded output never enters the alternate screen")
	}

	process.feed(asking[:cut])
	process.feed(asking[cut:])
	log.waitForCount(t, 1)
}

func TestClaudeCodeOutsideTmuxStaysWithItsOwnReports(t *testing.T) {
	session, process, log := openWatchedShell(t)
	process.feed("\x1b]7501;?\a\x1b]7501;state=blocked:app=claude-code:kind=permission:msg=YXBwcm92ZQ\a")
	process.feed("Do you want to proceed?\r\n ❯ 1. Yes\r\n   2. No")
	changes := log.waitForCount(t, 1)
	if changes[0].Status.Message != "approve" {
		t.Fatalf("status = %+v, want what the program said", changes[0].Status)
	}

	process.feed("\x1b]7501;state=clear\a\r\n")
	if changes := log.waitForCount(t, 2); changes[1].Status != nil {
		t.Fatalf("status = %+v, want none: the same words stay on the normal screen", changes[1].Status)
	}
	if status := session.View().ProgramStatus; status != nil {
		t.Fatalf("session list shows %+v, want nothing", status)
	}
}
