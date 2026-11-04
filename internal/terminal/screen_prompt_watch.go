package terminal

import (
	"strings"
	"time"

	"sshc/third_party/vt"
)

// Claude Code reports a question over OSC 7501 only when the terminal answers its query, and tmux keeps the query to
// itself and answers the DA1 sent after it, so under tmux nothing is ever reported. The question is still drawn on the
// screen. While the alternate screen is shown, which tmux always does, the engine follows the output with an emulator
// of its own and keeps a blocked record for as long as a question reads there (claude_code_prompt.go). The record does
// the rest, as one from the program would: the session list, the notifications.
//
// This cannot use screenCheckpoint, which reads only what the replay buffer lets go and so is a whole buffer behind.
// A program that reports for itself is left to it, and output on the normal screen costs nothing, so Claude Code
// outside tmux is not read at all.

const (
	// screenPromptRecordID is the record a question read off the screen is kept under. Its App stays empty:
	// resumablePrograms reads that name, and a program under tmux does not need starting again when the connection
	// goes, it is still running there.
	screenPromptRecordID = "sshc-screen-prompt"
	// screenPromptSettle is how long the output must stop before the screen is read, so that a redraw is not read
	// halfway. tmux writes one in a burst, and it is shorter than the 0.6 s between the repaints Claude Code 2.1.318
	// keeps making during some questions, which would otherwise hold the read back.
	screenPromptSettle = 300 * time.Millisecond
	// screenPromptLongestWait bounds that wait: another pane printing without a pause must not hide a question.
	screenPromptLongestWait = 2 * time.Second
	// screenPromptGoneAfter is how long a question must stay off the screen before its record is dropped. A resize or
	// a slow line can leave a redraw half done past screenPromptSettle, and each return would notify again.
	screenPromptGoneAfter = time.Second
	// enterAlternateScreen starts a new emulator on the screen the output just entered. The sequence that entered it
	// may have ended in an earlier chunk.
	enterAlternateScreen = "\x1b[?1049h"
)

// screenPromptWatch is one session's reading of its screen. Session.mutex guards it.
type screenPromptWatch struct {
	// emulator follows the output while the alternate screen is shown, and is nil otherwise.
	emulator *vt.SnapshotEmulator
	// timer reads the screen when it fires.
	timer *time.Timer
	// unreadSince is when the first output not read yet came; zero when all of it is read.
	unreadSince time.Time
	// goneSince is when the question was first found off the screen; zero while it is there.
	goneSince time.Time
}

// stop ends the following. A read already on its way finds no emulator.
func (w *screenPromptWatch) stop() {
	if w.timer != nil {
		w.timer.Stop()
	}
	*w = screenPromptWatch{}
}

// stopFollowingScreenIfPanicked is deferred around every use of the emulator, with Session.mutex held. The output comes
// from the far side, so a sequence the emulator cannot take is something a server can send. A question nobody is told
// about is a small loss beside the engine going down with every session.
func (s *Session) stopFollowingScreenIfPanicked() {
	if recover() == nil {
		return
	}
	s.screenPrompt.stop()
	s.programStatus.clear(screenPromptRecordID)
	s.reportBlockedProgramLocked()
}

// resizeFollowedScreenLocked keeps the emulator at the session's size.
func (s *Session) resizeFollowedScreenLocked(size Size) {
	defer s.stopFollowingScreenIfPanicked()
	if s.screenPrompt.emulator != nil {
		s.screenPrompt.emulator.Resize(int(size.Cols), int(size.Rows))
	}
}

// newAlternateScreenObserver tells when the output enters the alternate screen. The emulator sees it leave.
func newAlternateScreenObserver(entered func()) *controlSequenceScanner {
	return &controlSequenceScanner{sequence: func(parameters string, final byte) {
		modes, private := strings.CutPrefix(parameters, "?")
		if !private || final != 'h' {
			return
		}
		for _, mode := range strings.Split(modes, ";") {
			if mode == "47" || mode == "1047" || mode == "1049" {
				entered()
				return
			}
		}
	}}
}

// startFollowingScreen makes the emulator, in the generation whose output entered the alternate screen.
func (s *Session) startFollowingScreen(generation uint64) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	defer s.stopFollowingScreenIfPanicked()
	if generation != s.generation || s.exited != nil || s.screenPrompt.emulator != nil {
		return
	}
	s.screenPrompt.emulator = vt.NewSnapshotEmulator(int(s.size.Cols), int(s.size.Rows))
	_, _ = s.screenPrompt.emulator.WriteString(enterAlternateScreen)
}

// followScreenOutput gives the emulator a chunk of the program's output.
func (s *Session) followScreenOutput(generation uint64, chunk []byte) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	defer s.stopFollowingScreenIfPanicked()
	watch := &s.screenPrompt
	if generation != s.generation || watch.emulator == nil {
		return
	}
	_, _ = watch.emulator.Write(chunk)
	if !watch.emulator.AltScreenShown() {
		// The program left, or tmux detached: there is no longer a screen to read a question from.
		watch.stop()
		s.programStatus.clear(screenPromptRecordID)
		s.reportBlockedProgramLocked()
		return
	}
	now := time.Now()
	if watch.unreadSince.IsZero() {
		watch.unreadSince = now
	}
	wait := min(screenPromptSettle, screenPromptLongestWait-now.Sub(watch.unreadSince))
	if watch.timer == nil {
		watch.timer = time.AfterFunc(wait, s.readScreenPrompt)
	} else {
		watch.timer.Reset(wait)
	}
}

// readScreenPrompt makes the record say what the screen says.
func (s *Session) readScreenPrompt() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	defer s.stopFollowingScreenIfPanicked()
	watch := &s.screenPrompt
	if watch.emulator == nil {
		return
	}
	watch.unreadSince = time.Time{}
	if s.programStatus.hasLiveReport() {
		s.programStatus.clear(screenPromptRecordID)
	} else if kind, asking := claudeCodePromptOnScreen(watch.emulator.ShownCells()); asking {
		watch.goneSince = time.Time{}
		s.keepScreenPromptRecordLocked(kind)
	} else {
		s.dropScreenPromptRecordOnceGoneLocked()
	}
	// Also after a reconnect, which drops the records without saying so (reportBlockedProgramLocked).
	s.reportBlockedProgramLocked()
}

// keepScreenPromptRecordLocked sets the record unless it already says so: a new report would have a new Version, and
// the question would count as one not seen yet.
func (s *Session) keepScreenPromptRecordLocked(kind ProgramStatusKind) {
	if s.programStatus.byID[screenPromptRecordID].Kind == kind {
		return
	}
	s.programStatus.apply(programStatusReport{
		id:     screenPromptRecordID,
		status: ProgramStatus{State: ProgramStatusBlocked, Kind: kind},
	})
}

// dropScreenPromptRecordOnceGoneLocked drops the record once the question has stayed off the screen, and otherwise
// looks again then, since a question that is really gone may bring no more output.
func (s *Session) dropScreenPromptRecordOnceGoneLocked() {
	watch := &s.screenPrompt
	if _, kept := s.programStatus.byID[screenPromptRecordID]; !kept {
		return
	}
	now := time.Now()
	if watch.goneSince.IsZero() {
		watch.goneSince = now
	}
	if remaining := screenPromptGoneAfter - now.Sub(watch.goneSince); remaining > 0 {
		watch.timer.Reset(remaining)
		return
	}
	watch.goneSince = time.Time{}
	s.programStatus.clear(screenPromptRecordID)
}
