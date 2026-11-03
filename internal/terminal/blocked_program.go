package terminal

// A program blocked on the person (OSC 7501 state=blocked) needs them even when
// they are not looking at sshc. Android raises its notification from here, in
// the engine: the page's JavaScript may be stopped while the app is in the
// background, but the engine's foreground service keeps reading the output.

// BlockedProgramChange says that a session's program started or stopped waiting
// for the person. It follows the session as a whole: a second record of the
// same session going blocked is no news, and neither is a blocked program
// reporting again.
type BlockedProgramChange struct {
	SessionID string
	// SessionAlias is the connection the session runs on, "" for a local shell.
	SessionAlias string
	// SessionTitle is the session's name as the session list shows it.
	SessionTitle string
	// Status is the record the session list shows, or nil once nothing waits.
	Status *ProgramStatus
}

// reportBlockedProgramLocked tells programBlocked what changed since it last
// heard. It is not called when a reconnect drops the records: the program may
// well still wait (under tmux, say), so a notification saying so stays until
// the new process reports otherwise.
func (s *Session) reportBlockedProgramLocked() {
	if s.programBlocked == nil {
		return
	}
	status := s.programStatus.summary()
	if s.exited != nil || status == nil || status.State != ProgramStatusBlocked {
		status = nil
	}
	blocked := status != nil
	if blocked == s.blockedReported {
		return
	}
	s.blockedReported = blocked
	s.programBlocked(BlockedProgramChange{SessionID: s.id, SessionAlias: s.alias, SessionTitle: s.title, Status: status})
}
