package terminal

import "time"

// shellReports は、今のシェル（process の1つの世代）が報告してきたことである。シェルが替わったら丸ごと
// 捨てるので、シェルごとの報告を足すときはここに欄を足す。捨て忘れが起きない。
type shellReports struct {
	// title は、最後に届いた OSC 0/1/2 の名前である。ブラウザも同じシーケンスを自分で当てるが、どの画面も
	// 同じペインの名前を出すよう、エンジンも覚える。
	title string
	// titleGroup は、title が届いたときに前面で走っていたプロセスグループである。シェル自身か、エンジンから
	// 見えないプログラムが送ったときは 0。running_command_title.go。
	titleGroup int
	// startDirectory は、このシェルが最初に報告したフォルダである。working_directory.go。
	startDirectory string
	// programProgress は、プログラムの OSC 9;4 の報告である。program_progress.go。
	programProgress *ProgramProgress
	// commandStartedAt は、このシェルが最後にコマンドの開始（OSC 133;C）を報告した時刻である。走っている
	// コマンドが無ければゼロ。command_finished.go。
	commandStartedAt time.Time
	// visibilityReports は、プログラムが可視の報告（モード 2033）を求めたかである。visibility_reports.go。
	visibilityReports bool
}

// forgetShellReportsLocked は、前のシェルが報告したことを捨てる。前のシェルの名前がペインに残らないよう、
// 名前も決め直す。呼び出し側が mutex を持つ。
func (s *Session) forgetShellReportsLocked() {
	titled := s.shell.title != ""
	s.shell = shellReports{}
	if titled {
		s.recomputeTitleLocked()
	}
}
