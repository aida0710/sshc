package terminal

// 利用者を待っているプログラム（OSC 7501 の state=blocked）は、利用者が sshc を見ていないときにも、利用者を
// 必要とする。Android は、通知をここ（エンジン）から出す。アプリが背後にあるあいだ、ページの JavaScript は
// 止められることがあるが、エンジンのフォアグラウンドサービスは出力を読み続ける。

// BlockedProgramChange は、セッションのプログラムが利用者を待ち始めたか、待ち終えたことを表す。セッション
// 全体について伝える。同じセッションの2つめの記録が blocked になっても知らせず、blocked のプログラムが
// もう一度報告しても知らせない。
type BlockedProgramChange struct {
	SessionID string
	// SessionAlias は、セッションが動いている接続である。ローカルのシェルでは ""。
	SessionAlias string
	// SessionTitle は、セッションの一覧に出る形の、セッションの名前である。
	SessionTitle string
	// Status は、セッションの一覧に出す記録である。待っているものが無くなったら nil。
	Status *ProgramStatus
}

// reportBlockedProgramLocked は、最後に知らせてから変わったことを programBlocked に伝える。再接続が記録を
// 捨てるときには呼ばない。プログラムは（たとえば tmux の下で）まだ待っているかもしれないので、そう伝える
// 通知は、新しい process が別のことを報告するまで残す。
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
