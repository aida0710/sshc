package terminal

import "bytes"

// プログラムの走っていたシェルが新しいシェルに替わると、そのプログラムは無くなる。ただし、やめたところから
// 続けられるものがある。resumablePrograms は、それらを、OSC 7501 で名乗る名前で挙げる。
var resumablePrograms = map[string]struct {
	// name は、利用者が知っているプログラムの名前である。
	name string
	// command は、走っていたフォルダで、やめたところから始める。
	command string
}{
	// Claude Code は、起動から終了まで記録を持ち続ける（2.1.304 で確認）。
	"claude-code": {name: "Claude Code", command: "claude --continue"},
}

// ResumeHint は、起動し直すプログラムについて、セッションの View が伝えることである。
type ResumeHint struct {
	// Program は、利用者が知っているプログラムの名前である。
	Program string
	// Command は、新しいシェルに打ち込むものである。
	Command string
}

// resumableApp は、記録の中の、起動し直せるプログラムの名前を返す。無ければ ""。
func (r *programStatusRecords) resumableApp() string {
	for id := range r.byID {
		app := r.appOf(id)
		if _, resumable := resumablePrograms[app]; resumable {
			return app
		}
	}
	return ""
}

// noteProgramToResumeLocked は、新しい process が古いものに替わるときに呼ぶ。そのとき記録は、そこで何が
// 走っていたかをまだ言っている。前のエンジンの記録が挙げたプログラム（Spec.ResumeProgram）は、新しいシェルが
// 立ち上がるまで残す。
//
// まだ待っている案内はそのままにする。無くなったシェルは何も走らせておらず、それが報告したのはログインした
// 場所である。1度も動かなかったシェルも、プログラムについて何も言わない。1行の `cd dir && claude` では、
// dir を報告するプロンプトが出ないためである。
func (s *Session) noteProgramToResumeLocked() {
	if s.resumeProgram != "" {
		return
	}
	if app := s.programStatus.resumableApp(); app != "" {
		s.resumeProgram = app
		s.resumeDirectory = ""
		s.resumeStartsInDirectory = false
		if s.workingDirectory != s.shell.startDirectory {
			s.resumeDirectory = s.workingDirectory
		}
	}
}

// resumeHintLocked は、新しいシェルに打ち込めるようになったあと、1回押すだけでプログラムを起動し直す方法を
// 伝える。フォルダをもとに続きを探すプログラムは、ほかの場所で走らせると、続けるものを見つけられず、その
// フォルダを信頼するかを尋ねる。そのため、フォルダが分からなければ、announceProgramToResume が書くものしか
// 出さない。
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

// resumeCommandLocked は、打ち込むコマンドと、それがプログラムの走っていたフォルダで走るかを返す。そのフォルダで
// 走らせるには、フォルダが分かっていて、どのログインシェルでも同じ書き方で書けなければならない。再起動の
// あとに開き直したセッションは、はじめからそこで始まる。
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

// announceProgramToResume は、プログラムを起動し直す方法を、新しいシェルのターミナルに書く。出力と一緒に
// 残るので、どのブラウザでも、View の案内が消えたあとでも、同じように読める。
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

// forgetProgramToResumeOnEnter は、利用者が新しいシェルで何かを実行したら、案内を捨てる。案内は役目を終えたか、
// 利用者が別のことを選んだ。数えるのは Enter だけである。ターミナルの自動の返事も入力として届くためである。
func (s *Session) forgetProgramToResumeOnEnter(input []byte) {
	if bytes.IndexByte(input, '\r') < 0 {
		return
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	// 案内のコマンドは、プログラムを起動するのと同じ行でシェルを移動させる。シェルには、移動を報告する
	// プロンプトが出ない。
	if hint := s.resumeHintLocked(); hint != nil && s.resumeDirectory != "" && bytes.Contains(input, []byte(hint.Command)) {
		s.workingDirectory = s.resumeDirectory
	}
	s.resumeProgram, s.resumeDirectory = "", ""
}

// programToResumeLocked は、次のエンジンへの記録に書くものである。今走っているプログラムか、まだ起動し直しを
// 待っているプログラムを返す。
func (s *Session) programToResumeLocked() string {
	if app := s.programStatus.resumableApp(); app != "" {
		return app
	}
	return s.resumeProgram
}

// isResumableProgram は、前のエンジンの記録から読んだ名前が、このエンジンが起動し直せるプログラムの名前かを返す。
func isResumableProgram(app string) bool {
	_, resumable := resumablePrograms[app]
	return resumable
}
