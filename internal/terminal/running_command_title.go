package terminal

import "time"

// 一覧とペインの名前に、ローカルのシェルが前面で動かしているコマンドの名前を足す。
// `sleep 31` のあいだは、シェルが送ったタイトルの前に名前を置き「sleep · aida@r100: ~」と出す。
// 名前を前に置くのは、一覧の幅で後ろが切れても、何が動いているかが残るようにするためである。

// runningCommandTitleDelay は、コマンドの名前を出すまでに、そのコマンドが動いている
// べき長さである。`ls` のようにすぐ終わるコマンドで、名前がちらつかないようにする。
const runningCommandTitleDelay = time.Second

// runningCommandTitleSeparator は、コマンドの名前とシェルが送ったタイトルのあいだに置く。
const runningCommandTitleSeparator = " · "

// foregroundProgram は、シェルが前面で動かしているプログラムを Process に聞く。
func (s *Session) foregroundProgram() (ForegroundProgram, bool) {
	s.mutex.Lock()
	reporter, ok := s.process.(ForegroundReporter)
	connected := s.state == StateConnected && s.exited == nil && !s.stoppedLocked()
	s.mutex.Unlock()
	if !ok || !connected {
		return ForegroundProgram{}, false
	}
	return reporter.ForegroundProgram()
}

// displayTitleLocked は、一覧とペインに出す名前を返す。running は、シェルが前面で
// プログラムを動かしているかである。
func (s *Session) displayTitleLocked(program ForegroundProgram, running bool) string {
	switch {
	case !running || program.Running < runningCommandTitleDelay || s.titlePinned:
		return s.title
	case s.terminalTitle != "" && s.terminalTitleGroup == program.Group:
		// 動いているプログラムが自分で付けた名前（Claude Code、vim）は、そのまま出す。
		return s.title
	case s.terminalTitle != "" && s.terminalTitleGroup == 0:
		// シェルがプロンプトで送った名前は残し、何が動いているかを前に足す。
		return program.Name + runningCommandTitleSeparator + s.terminalTitle
	default:
		// シェルが名前を送っていないか、前に動いたプログラムの名前が残っているだけなら、
		// コマンドの名前だけを出す。シェル名（bash）を足しても、分かることは増えない。
		return program.Name
	}
}
