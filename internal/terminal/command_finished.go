package terminal

import "time"

// 長いコマンドの終わりは通知として数える。誰も見ていないセッションにも、未読の印とデスクトップの通知が
// 付くようにするためである。時間を測るのは、ブラウザではなくエンジンである。別のワークスペースのセッションや、
// 分割表示の外のセッションは、どのブラウザにもターミナルが描かれていない。

// LongCommand は、終わりを通知するコマンドの、いちばん短い長さである。これより短ければ、利用者はまだ見ている
// はずで、すぐ終わるコマンドのたびにセッションが未読になってしまう。
const LongCommand = 30 * time.Second

// FinishedCommand は、シェルが終わったと報告した、LongCommand 以上かかったコマンドである。
type FinishedCommand struct {
	Duration time.Duration
	// ExitCode は、シェルが終了コードを報告しなかったとき nil である。
	ExitCode *int
}

func (s *Session) acceptCommandStarted(generation uint64, at time.Time) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if generation != s.generation || s.exited != nil {
		return
	}
	s.shell.commandStartedAt = at
}

// acceptCommandFinished は、長いコマンドの終わりを通知する。シェルの最初のプロンプトのような、始まりの無い
// 終わりは、コマンドではない。
func (s *Session) acceptCommandFinished(generation uint64, exitCode *int, at time.Time) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if generation != s.generation || s.exited != nil {
		return
	}
	started := s.shell.commandStartedAt
	s.shell.commandStartedAt = time.Time{}
	if started.IsZero() || at.Sub(started) < LongCommand {
		return
	}
	s.notificationVersion++
	s.lastNotification = &Notification{
		OccurredAt:      at,
		FinishedCommand: &FinishedCommand{Duration: at.Sub(started), ExitCode: exitCode},
	}
}
