package terminal

// Reopening は、セッションを開き直すのに要るものである。同じ接続かローカルのシェルプロファイルを、同じ
// ワークスペースで、最後に作業していた場所から始める。
type Reopening struct {
	Kind         Kind
	Alias        string
	ShellProfile string
	Workspace    string
	// PinnedTitle は、利用者がセッションに付けた名前である。付けていなければ ""。
	PinnedTitle string
	// WorkingDirectory は、セッションが1度も報告しなかったとき "" である。
	WorkingDirectory string
	// TmuxSession は、接続が tmux を使うときに、その接続が開く tmux のセッションの名前である。
	TmuxSession string
	// ResumeProgram は、新しいシェルで起動し直せるプログラムの、OSC 7501 の名前である。無ければ ""。
	ResumeProgram string
}

// Reopening は、開き直すために、このセッションを書き表す。
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

// Reopenings は、まだ使っているセッションを、開いた順に書き表す。利用者が閉じたセッションと、プログラムが
// 終わったセッションは含めない。接続を失った SSH のセッションは、再接続を待っていても、あきらめていても、
// まだ使っているものとする。利用者が手で再接続するために残したものである。
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
