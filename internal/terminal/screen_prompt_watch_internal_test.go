package terminal

import (
	"testing"
	"time"

	"sshc/third_party/vt"
)

const shownBashQuestion = "\x1b[?1049h\x1b[HDo you want to proceed?\r\n ❯ 1. Yes\r\n   2. No"

// sessionFollowingScreen is a session reading its screen, with the timer never firing, so a test decides when it reads.
func sessionFollowingScreen(t *testing.T, output string) *Session {
	t.Helper()
	session := &Session{size: DefaultSize()}
	session.startFollowingScreen(0)
	if session.screenPrompt.emulator == nil {
		t.Fatal("the session is not following its screen")
	}
	_, _ = session.screenPrompt.emulator.WriteString(output)
	session.screenPrompt.timer = time.NewTimer(time.Hour)
	t.Cleanup(session.screenPrompt.stop)
	return session
}

func TestAQuestionOnScreenIsKeptAsABlockedRecordWithTheSameVersion(t *testing.T) {
	session := sessionFollowingScreen(t, shownBashQuestion)
	session.readScreenPrompt()
	status := session.programStatus.summary()
	if status == nil || status.State != ProgramStatusBlocked || status.Kind != ProgramStatusPermission {
		t.Fatalf("status = %+v, want blocked on a permission", status)
	}

	session.readScreenPrompt()
	if again := session.programStatus.summary(); again.Version != status.Version {
		t.Fatalf("version went from %d to %d with the same question on screen", status.Version, again.Version)
	}
}

func TestARecordReadFromTheScreenOffersNoProgramToResume(t *testing.T) {
	session := sessionFollowingScreen(t, shownBashQuestion)
	session.readScreenPrompt()
	if app := session.programStatus.resumableApp(); app != "" {
		t.Fatalf("resumableApp() = %q, want none: the program is still running under tmux", app)
	}
}

func TestAQuestionOffTheScreenStaysUntilItHasBeenGoneForAWhile(t *testing.T) {
	session := sessionFollowingScreen(t, shownBashQuestion)
	session.readScreenPrompt()

	_, _ = session.screenPrompt.emulator.WriteString("\x1b[2J")
	session.readScreenPrompt()
	if session.programStatus.summary() == nil {
		t.Fatal("the record went with the first read that did not find the question")
	}

	// It came back, as after a redraw that was slow: the wait starts over.
	_, _ = session.screenPrompt.emulator.WriteString("\x1b[H" + shownBashQuestion)
	session.readScreenPrompt()
	if !session.screenPrompt.goneSince.IsZero() {
		t.Fatal("goneSince stayed set with the question back on screen")
	}

	_, _ = session.screenPrompt.emulator.WriteString("\x1b[2J")
	session.readScreenPrompt()
	session.screenPrompt.goneSince = time.Now().Add(-screenPromptGoneAfter)
	session.readScreenPrompt()
	if status := session.programStatus.summary(); status != nil {
		t.Fatalf("status = %+v, want none once the question has stayed away", status)
	}
}

func TestAProgramThatReportsForItselfIsNotRead(t *testing.T) {
	for _, state := range []ProgramStatusState{ProgramStatusIdle, ProgramStatusWorking} {
		session := sessionFollowingScreen(t, shownBashQuestion)
		session.programStatus.apply(programStatusReport{status: ProgramStatus{State: state}})
		session.readScreenPrompt()
		if status := session.programStatus.summary(); status.State != state {
			t.Fatalf("status = %+v with the program reporting %s", status, state)
		}
	}
}

func TestAResultLeftByAProgramThatWentDoesNotStopTheReading(t *testing.T) {
	session := sessionFollowingScreen(t, shownBashQuestion)
	session.programStatus.apply(programStatusReport{status: ProgramStatus{State: ProgramStatusDone}})
	session.readScreenPrompt()
	if status := session.programStatus.summary(); status.State != ProgramStatusBlocked {
		t.Fatalf("status = %+v, want the question read from the screen", status)
	}
}

func TestAProgramStartingToReportTakesOverFromTheScreen(t *testing.T) {
	session := sessionFollowingScreen(t, shownBashQuestion)
	session.readScreenPrompt()

	session.programStatus.apply(programStatusReport{status: ProgramStatus{State: ProgramStatusWorking}})
	session.readScreenPrompt()
	if status := session.programStatus.summary(); status.State != ProgramStatusWorking {
		t.Fatalf("status = %+v, want only what the program reports", status)
	}
}

func TestTheOutputIsFollowedOnlyOnTheAlternateScreen(t *testing.T) {
	session := &Session{size: DefaultSize()}
	entered := newAlternateScreenObserver(func() { session.startFollowingScreen(0) })
	follow := func(output string) {
		entered.Observe([]byte(output))
		session.followScreenOutput(0, []byte(output))
	}

	follow("Do you want to proceed?\r\n ❯ 1. Yes\r\n   2. No\r\n$ \x1b[?2004h")
	if session.screenPrompt.emulator != nil {
		t.Fatal("followed the normal screen")
	}

	follow("\x1b[?10")
	follow("49h")
	if session.screenPrompt.emulator == nil {
		t.Fatal("did not follow the alternate screen entered across two chunks")
	}
	t.Cleanup(session.screenPrompt.stop)

	follow("\x1b[?1049l")
	if session.screenPrompt.emulator != nil {
		t.Fatal("still following after the program left the alternate screen")
	}
}

func TestAResizeReachesTheEmulator(t *testing.T) {
	session := sessionFollowingScreen(t, "")
	session.screenPrompt.resized(Size{Cols: 40, Rows: 30})
	cells := session.screenPrompt.emulator.ShownCells()
	if len(cells) != 30 || len(cells[0]) != 40 {
		t.Fatalf("screen is %d rows of %d, want 30 rows of 40", len(cells), len(cells[0]))
	}
}

// Only the emulator's alternate screen is of use here, so make sure a fresh one starts on it.
func TestEnterAlternateScreenShowsTheAlternateScreen(t *testing.T) {
	emulator := vt.NewSnapshotEmulator(10, 2)
	_, _ = emulator.WriteString(enterAlternateScreen)
	if !emulator.AltScreenShown() {
		t.Fatal("not on the alternate screen")
	}
}
