package terminal

// Reopening is what it takes to open a session again: the same connection or
// local shell profile, in the same workspace, starting where it last worked.
type Reopening struct {
	Kind         Kind
	Alias        string
	ShellProfile string
	Workspace    string
	// PinnedTitle is the name the user gave the session, or "" when it has none.
	PinnedTitle string
	// WorkingDirectory is "" when the session never reported one.
	WorkingDirectory string
	// TmuxSession is the name of the tmux session its connection opens, if it uses tmux.
	TmuxSession string
	// ResumeProgram is the OSC 7501 name of a program that can be started again
	// in the new shell, or "".
	ResumeProgram string
}

// Reopening describes this session for opening it again.
func (s *Session) Reopening() Reopening {
	directory := s.WorkingDirectory()
	s.mutex.Lock()
	defer s.mutex.Unlock()
	reopening := Reopening{
		Kind: s.kind, Alias: s.alias, ShellProfile: s.shellProfile,
		Workspace: s.workspace, WorkingDirectory: directory,
		TmuxSession: s.tmuxSession, ResumeProgram: s.programToResumeLocked(),
	}
	if s.titlePinned {
		reopening.PinnedTitle = s.title
	}
	return reopening
}

// Reopenings describes, in the order they were opened, the sessions still in
// use. A session the user closed or whose program exited is left out. An SSH
// session that lost its connection is still in use, whether it is waiting to
// reconnect or gave up: the user kept it to reconnect by hand.
func (r *Registry) Reopenings() []Reopening {
	r.mutex.Lock()
	sessions := append([]*Session(nil), r.sessions...)
	r.mutex.Unlock()
	reopenings := make([]Reopening, 0, len(sessions))
	for _, session := range sessions {
		if !session.inUse() {
			continue
		}
		reopenings = append(reopenings, session.Reopening())
	}
	return reopenings
}

func (s *Session) inUse() bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.discarded {
		return false
	}
	if s.exited == nil {
		return true
	}
	switch s.problem {
	case ProblemReconnectFailed, ProblemReconnectStopped, ProblemReconnectExhausted:
		return true
	}
	return false
}
