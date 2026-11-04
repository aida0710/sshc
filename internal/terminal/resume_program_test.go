package terminal_test

import (
	"context"
	"strings"
	"testing"

	"sshc/internal/terminal"
	"sshc/internal/testwait"
)

const claudeCodeReport = "\x1b]7501;state=idle:app=claude-code\x1b\\"

// shellMovesToSource は、ログインしたシェルが ~ を言い、cd して /home/me/src を言う出力である。
const shellMovesToSource = "\x1b]7;file://host/home/me\x1b\\\x1b]7;file://host/home/me/src\x1b\\"

// openLosing は、最初の接続に first を返し、次からは新しい Process を返すセッションを開く。
// 返す spy は、開いた回数を言う。
func openLosing(t *testing.T, first *fakeProcess) (*terminal.Session, *tmuxNameSpy) {
	t.Helper()
	spy := &tmuxNameSpy{process: func(open int) terminal.Process {
		if open == 1 {
			return first
		}
		return newFakeProcess()
	}}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "web", Title: "web", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}
	testwait.Until(t, func() bool { return session.View().State == terminal.StateConnected })
	return session, spy
}

func TestANewShellInPlaceOfOneThatRanClaudeCodeSaysHowToContinue(t *testing.T) {
	first := newFakeProcess()
	session, _ := openLosing(t, first)
	first.feed(shellMovesToSource)
	first.feed(claudeCodeReport)
	testwait.Until(t, func() bool { return session.View().ProgramStatus != nil })

	first.exit(transportLost)
	testwait.Until(t, func() bool { return session.View().Resume != nil })

	want := terminal.ResumeHint{Program: "Claude Code", Command: "cd -- '/home/me/src' && claude --continue"}
	if got := *session.View().Resume; got != want {
		t.Errorf("Resume = %+v, want %+v", got, want)
	}
	if !strings.Contains(string(snapshotOf(session)), want.Command) {
		t.Errorf("the terminal does not say it: %q", snapshotOf(session))
	}
}

func TestAShellWithNoKnownDirectoryOffersNoTapAndSaysWhereToRunTheCommand(t *testing.T) {
	first := newFakeProcess()
	session, spy := openLosing(t, first)
	first.feed(claudeCodeReport)
	testwait.Until(t, func() bool { return session.View().ProgramStatus != nil })

	first.exit(transportLost)
	testwait.Until(t, func() bool { return len(spy.seen()) == 2 && session.View().State == terminal.StateConnected })
	said := "動かしていたフォルダで claude --continue を実行してください"
	testwait.Until(t, func() bool { return strings.Contains(string(snapshotOf(session)), said) })
	if hint := session.View().Resume; hint != nil {
		t.Errorf("Resume = %+v, but running it here would miss the conversation", hint)
	}
}

func TestAShellThatNeverMovedDoesNotTellWhereTheProgramRan(t *testing.T) {
	first := newFakeProcess()
	session, spy := openLosing(t, first)
	first.feed("\x1b]7;file://host/home/me\x1b\\")
	first.feed(claudeCodeReport) // `cd src && claude` を1行で打った場合、シェルが言う機会は無い。
	testwait.Until(t, func() bool { return session.View().ProgramStatus != nil })

	first.exit(transportLost)
	testwait.Until(t, func() bool { return len(spy.seen()) == 2 && session.View().State == terminal.StateConnected })
	testwait.Until(t, func() bool {
		return strings.Contains(string(snapshotOf(session)), "動かしていたフォルダで claude --continue")
	})
	if hint := session.View().Resume; hint != nil {
		t.Errorf("Resume = %+v", hint)
	}
}

func TestASecondLossBeforeAnythingRanKeepsTheDirectoryOfTheHint(t *testing.T) {
	first, second := newFakeProcess(), newFakeProcess()
	spy := &tmuxNameSpy{process: func(open int) terminal.Process {
		switch open {
		case 1:
			return first
		case 2:
			return second
		}
		return newFakeProcess()
	}}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{Kind: terminal.KindSSH, Alias: "web", Title: "web", Open: spy.open})
	if err != nil {
		t.Fatal(err)
	}
	testwait.Until(t, func() bool { return session.View().State == terminal.StateConnected })
	first.feed(shellMovesToSource)
	first.feed("\x1b]7501;state=done:app=claude-code\x1b\\") // done の記録は次のシェルへ残る。
	testwait.Until(t, func() bool { return session.View().ProgramStatus != nil })
	first.exit(transportLost)
	testwait.Until(t, func() bool { return session.View().Resume != nil })

	second.feed("\x1b]7;file://host/home/me\x1b\\")
	testwait.Until(t, func() bool { return session.View().WorkingDirectory == "/home/me" })
	second.exit(transportLost)
	testwait.Until(t, func() bool { return len(spy.seen()) == 3 && session.View().Resume != nil })

	if got, want := session.View().Resume.Command, "cd -- '/home/me/src' && claude --continue"; got != want {
		t.Errorf("Command = %q, want %q", got, want)
	}
}

func TestRunningTheHintKeepsItsDirectoryForTheNextLoss(t *testing.T) {
	first, second := newFakeProcess(), newFakeProcess()
	spy := &tmuxNameSpy{process: func(open int) terminal.Process {
		switch open {
		case 1:
			return first
		case 2:
			return second
		}
		return newFakeProcess()
	}}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{Kind: terminal.KindSSH, Alias: "web", Title: "web", Open: spy.open})
	if err != nil {
		t.Fatal(err)
	}
	testwait.Until(t, func() bool { return session.View().State == terminal.StateConnected })
	first.feed(shellMovesToSource)
	first.feed(claudeCodeReport)
	testwait.Until(t, func() bool { return session.View().ProgramStatus != nil })
	first.exit(transportLost)
	testwait.Until(t, func() bool { return session.View().Resume != nil })
	command := session.View().Resume.Command

	second.feed("\x1b]7;file://host/home/me\x1b\\")
	testwait.Until(t, func() bool { return session.View().WorkingDirectory == "/home/me" })
	if _, err := session.Write([]byte(command + "\r")); err != nil {
		t.Fatal(err)
	}
	second.feed(claudeCodeReport)
	testwait.Until(t, func() bool { return session.View().ProgramStatus != nil })
	second.exit(transportLost)
	testwait.Until(t, func() bool { return len(spy.seen()) == 3 && session.View().Resume != nil })

	if got := session.View().Resume.Command; got != command {
		t.Errorf("Command = %q, want %q", got, command)
	}
}

func TestTheHintGoesOnceSomethingIsRunInTheNewShell(t *testing.T) {
	first := newFakeProcess()
	session, _ := openLosing(t, first)
	first.feed(shellMovesToSource)
	first.feed(claudeCodeReport)
	testwait.Until(t, func() bool { return session.View().ProgramStatus != nil })
	first.exit(transportLost)
	testwait.Until(t, func() bool { return session.View().Resume != nil })

	if _, err := session.Write([]byte("cl")); err != nil {
		t.Fatal(err)
	}
	if session.View().Resume == nil {
		t.Fatal("the hint went before anything was run")
	}
	if _, err := session.Write([]byte("aude --continue\r")); err != nil {
		t.Fatal(err)
	}
	if session.View().Resume != nil {
		t.Error("the hint stayed after the command was run")
	}
}

func TestAShellThatRanNoSuchProgramGetsNoHint(t *testing.T) {
	first := newFakeProcess()
	session, spy := openLosing(t, first)
	first.feed("\x1b]7501;state=working:app=cargo\x1b\\")
	testwait.Until(t, func() bool { return session.View().ProgramStatus != nil })

	first.exit(transportLost)
	testwait.Until(t, func() bool { return len(spy.seen()) == 2 && session.View().State == terminal.StateConnected })
	if hint := session.View().Resume; hint != nil {
		t.Errorf("Resume = %+v", hint)
	}
}

func TestAProgramThatClearedItsRecordLeftNothingToContinue(t *testing.T) {
	first := newFakeProcess()
	session, spy := openLosing(t, first)
	first.feed(claudeCodeReport)
	testwait.Until(t, func() bool { return session.View().ProgramStatus != nil })
	first.feed("\x1b]7501;state=clear\x1b\\")
	testwait.Until(t, func() bool { return session.View().ProgramStatus == nil })

	first.exit(transportLost)
	testwait.Until(t, func() bool { return len(spy.seen()) == 2 && session.View().State == terminal.StateConnected })
	if hint := session.View().Resume; hint != nil {
		t.Errorf("Resume = %+v", hint)
	}
}

func TestTheRecordForTheNextEngineNamesTheProgramAndASessionOpenedFromItSaysHowToContinue(t *testing.T) {
	first := newFakeProcess()
	session, _ := openLosing(t, first)
	first.feed(claudeCodeReport)
	testwait.Until(t, func() bool { return session.View().ProgramStatus != nil })
	if got := session.Reopening().ResumeProgram; got != "claude-code" {
		t.Fatalf("ResumeProgram = %q", got)
	}

	registry, _ := newFastRegistry()
	reopened, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "web", Title: "web", ResumeProgram: "claude-code", ResumeStartsInDirectory: true,
		Open: func(context.Context, terminal.Size) (terminal.Process, error) { return newFakeProcess(), nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	testwait.Until(t, func() bool { return reopened.View().Resume != nil })
	// 開き直しは保存した場所でシェルを始めるので、cd は要らない。
	if got := reopened.View().Resume.Command; got != "claude --continue" {
		t.Errorf("Command = %q", got)
	}
}

func TestAReturnToARunningTmuxSessionHasNothingToContinue(t *testing.T) {
	first := newFakeProcess()
	spy := &tmuxNameSpy{process: func(open int) terminal.Process {
		if open == 1 {
			return first
		}
		return &tmuxProcess{fakeProcess: newFakeProcess(), resumed: true}
	}}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "web", Title: "web", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}
	first.feed(claudeCodeReport)
	testwait.Until(t, func() bool { return session.View().ProgramStatus != nil })
	first.exit(transportLost)
	testwait.Until(t, func() bool { return strings.Contains(string(snapshotOf(session)), "再接続しました") })
	if hint := session.View().Resume; hint != nil {
		t.Errorf("Resume = %+v", hint)
	}
}
