package terminal

import (
	"testing"
	"time"
)

// foregroundStub is a local shell's Process as far as the title needs one: it
// names the program in the foreground, or none. Other Process methods are not
// called by these tests.
type foregroundStub struct {
	Process
	program ForegroundProgram
	running bool
}

func (p *foregroundStub) ForegroundProgram() (ForegroundProgram, bool) { return p.program, p.running }

func (p *foregroundStub) runs(group int, name string) {
	p.program, p.running = ForegroundProgram{Group: group, Name: name}, true
}

// keepsRunning lets the program in the foreground run for longer.
func (p *foregroundStub) keepsRunning(duration time.Duration) { p.program.Running += duration }

func (p *foregroundStub) returnsToPrompt() { p.program, p.running = ForegroundProgram{}, false }

type localShellForTitle struct {
	session *Session
	process *foregroundStub
}

func newLocalShellForTitle() *localShellForTitle {
	shell := &localShellForTitle{process: &foregroundStub{}}
	shell.session = &Session{
		id: "s1", kind: KindShell, title: "bash", fallbackTitle: "bash",
		generation: 1, state: StateConnected, buffer: NewRing(32), process: shell.process,
	}
	shell.session.recomputeTitleLocked()
	return shell
}

func (shell *localShellForTitle) wait(duration time.Duration) { shell.process.keepsRunning(duration) }

func (shell *localShellForTitle) title() string {
	return shell.session.View().Presentation.DisplayTitle
}

func TestRunningCommandGoesBeforeTheTitleTheShellSent(t *testing.T) {
	shell := newLocalShellForTitle()
	shell.session.acceptTitle(1, "aida@r540: ~")
	shell.process.runs(4242, "sleep")
	if got := shell.title(); got != "aida@r540: ~" {
		t.Fatalf("title as the command starts = %q, want the shell's title until it has run a second", got)
	}
	shell.wait(runningCommandTitleDelay)
	view := shell.session.View()
	if view.Presentation.DisplayTitle != "sleep · aida@r540: ~" || view.Title != "sleep · aida@r540: ~" {
		t.Fatalf("title while sleep runs = %q (Title %q)", view.Presentation.DisplayTitle, view.Title)
	}
	if view.Presentation.TitleSource != TitleTerminal {
		t.Fatalf("title source = %q, want the shell's title to stay the source", view.Presentation.TitleSource)
	}
	shell.process.returnsToPrompt()
	if got := shell.title(); got != "aida@r540: ~" {
		t.Fatalf("title back at the prompt = %q", got)
	}
}

func TestRunningCommandAloneWhenTheShellSentNoTitle(t *testing.T) {
	shell := newLocalShellForTitle()
	shell.process.runs(4242, "sleep")
	shell.wait(runningCommandTitleDelay)
	if got := shell.title(); got != "sleep" {
		t.Fatalf("title while sleep runs = %q, want the command alone rather than %q", got, "sleep · bash")
	}
}

func TestCommandThatEndsWithinASecondNeverShows(t *testing.T) {
	shell := newLocalShellForTitle()
	shell.session.acceptTitle(1, "aida@r540: ~")
	shell.process.runs(4242, "ls")
	shell.wait(runningCommandTitleDelay / 2)
	shell.process.returnsToPrompt()
	shell.process.runs(4243, "cat")
	shell.wait(runningCommandTitleDelay / 2)
	if got := shell.title(); got != "aida@r540: ~" {
		t.Fatalf("title after two short commands = %q, want the shell's title", got)
	}
}

func TestTitleTheRunningProgramSentIsShownAsItIs(t *testing.T) {
	shell := newLocalShellForTitle()
	shell.session.acceptTitle(1, "aida@r540: ~/projects/sshc")
	shell.process.runs(5000, "claude")
	shell.session.acceptTitle(1, "✳ API認証の修正")
	shell.wait(runningCommandTitleDelay)
	if got := shell.title(); got != "✳ API認証の修正" {
		t.Fatalf("title while Claude Code runs = %q, want its own title", got)
	}
}

func TestTitleAnEarlierProgramLeftIsNotPutAfterTheNextCommand(t *testing.T) {
	shell := newLocalShellForTitle()
	shell.process.runs(5000, "claude")
	shell.session.acceptTitle(1, "✳ API認証の修正")
	shell.process.returnsToPrompt()
	if got := shell.title(); got != "✳ API認証の修正" {
		t.Fatalf("title at the prompt after Claude Code = %q, want the title it left, as before", got)
	}
	shell.process.runs(5100, "sleep")
	shell.wait(runningCommandTitleDelay)
	if got := shell.title(); got != "sleep" {
		t.Fatalf("title while sleep runs = %q, want the command alone", got)
	}
}

func TestPinnedTitleHidesTheRunningCommand(t *testing.T) {
	shell := newLocalShellForTitle()
	if err := shell.session.Rename("build"); err != nil {
		t.Fatal(err)
	}
	shell.process.runs(4242, "make")
	shell.wait(runningCommandTitleDelay)
	if got := shell.title(); got != "build" {
		t.Fatalf("pinned title while make runs = %q", got)
	}
}
