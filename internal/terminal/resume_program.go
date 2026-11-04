package terminal

import "bytes"

// When a new shell takes the place of the one a program ran in, that program is
// gone, but some can pick up where they left off. resumablePrograms names those
// by the name they give themselves in OSC 7501.
var resumablePrograms = map[string]struct {
	// name is how the person knows the program.
	name string
	// command starts it where it left off, in the directory it ran in.
	command string
}{
	// Claude Code keeps a record from its start until it exits (seen in 2.1.304).
	"claude-code": {name: "Claude Code", command: "claude --continue"},
}

// ResumeHint is what the view of a session says about a program to start again.
type ResumeHint struct {
	// Program is the name the person knows the program by.
	Program string
	// Command is what to type into the new shell.
	Command string
}

// resumableApp returns the name of a program in the records that can be started
// again, or "".
func (r *programStatusRecords) resumableApp() string {
	for id := range r.byID {
		app := r.appOf(id)
		if _, resumable := resumablePrograms[app]; resumable {
			return app
		}
	}
	return ""
}

// noteProgramToResumeLocked is called as a new process replaces the old one,
// while the records still say what ran there. A program named by the record of
// the previous engine (Spec.ResumeProgram) stays until the new shell is up.
//
// A hint still waiting stays as it is: the shell that went ran nothing, and what
// it reported is where it logged in. A shell that never moved says nothing about
// the program either, since `cd dir && claude` on one line gives it no prompt to
// report dir from.
func (s *Session) noteProgramToResumeLocked() {
	if s.resumeProgram != "" {
		return
	}
	if app := s.programStatus.resumableApp(); app != "" {
		s.resumeProgram = app
		s.resumeDirectory = ""
		s.resumeStartsInDirectory = false
		if s.workingDirectory != s.shellStartDirectory {
			s.resumeDirectory = s.workingDirectory
		}
	}
}

// resumeHintLocked says how to start the program again with one tap, once the
// new shell is there to type into. Run anywhere else, a program that resumes by
// its directory finds nothing to continue and asks to trust that directory, so
// without the directory there is only what announceProgramToResume writes.
func (s *Session) resumeHintLocked() *ResumeHint {
	program, resumable := resumablePrograms[s.resumeProgram]
	if !resumable || s.state != StateConnected {
		return nil
	}
	command, inItsDirectory := s.resumeCommandLocked(program.command)
	if !inItsDirectory {
		return nil
	}
	return &ResumeHint{Program: program.name, Command: command}
}

// resumeCommandLocked is the command to type, and whether it runs in the
// directory the program ran in. That takes the directory being known and
// writable the same way in every login shell. A session opened again after a
// restart starts there already.
func (s *Session) resumeCommandLocked(command string) (typed string, inItsDirectory bool) {
	if s.resumeStartsInDirectory {
		return command, true
	}
	if s.resumeDirectory == "" {
		return command, false
	}
	change, ok := ChangeRemoteDirectoryCommand(s.resumeDirectory)
	if !ok {
		return command, false
	}
	return change + " && " + command, true
}

// announceProgramToResume writes how to start the program again into the new
// shell's terminal. It is kept with the output, so it reads the same for every
// browser and after the hint in the view is gone.
func (s *Session) announceProgramToResume() {
	s.mutex.Lock()
	announcement := s.resumeAnnouncementLocked()
	s.mutex.Unlock()
	if announcement != "" {
		s.publish([]byte(announcement))
	}
}

func (s *Session) resumeAnnouncementLocked() string {
	program, resumable := resumablePrograms[s.resumeProgram]
	if !resumable || s.state != StateConnected {
		return ""
	}
	command, inItsDirectory := s.resumeCommandLocked(program.command)
	where := " "
	if !inItsDirectory {
		where = "動かしていたフォルダで "
	}
	return "\r\n[sshc] 前のシェルで" + program.name + "が動いていました。続きから始めるには" + where + command + " を実行してください。\r\n"
}

// forgetProgramToResumeOnEnter drops the hint once the person has run something
// in the new shell: the hint has served, or they chose otherwise. Only an Enter
// counts, since a terminal's automatic replies also arrive as input.
func (s *Session) forgetProgramToResumeOnEnter(input []byte) {
	if bytes.IndexByte(input, '\r') < 0 {
		return
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	// The hint's command moves the shell on the same line that starts the program,
	// so the shell has no prompt to report it from.
	if hint := s.resumeHintLocked(); hint != nil && s.resumeDirectory != "" && bytes.Contains(input, []byte(hint.Command)) {
		s.workingDirectory = s.resumeDirectory
	}
	s.resumeProgram, s.resumeDirectory = "", ""
}

// programToResumeLocked is what the record for the next engine says: the
// program running now, or the one still waiting to be started again.
func (s *Session) programToResumeLocked() string {
	if app := s.programStatus.resumableApp(); app != "" {
		return app
	}
	return s.resumeProgram
}

// isResumableProgram reports whether a name read from a record of the previous
// engine names a program this one can start again.
func isResumableProgram(app string) bool {
	_, resumable := resumablePrograms[app]
	return resumable
}
