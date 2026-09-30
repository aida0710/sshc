package terminal

import (
	"context"
	"fmt"
	"time"
)

// 落ちた SSH の輸送を自動で繋ぎ直す処理と、終了したセッションを利用者の操作で
// 同じ ID のまま繋ぎ直す処理を扱う。

// MaxReconnects は、自動再接続を試す回数の既定値で、設定で選べる最大の回数でもある。
const MaxReconnects = 5

// ReconnectSettled は、再接続予算を戻してよい連続稼働時間である。短時間に
// 切断を繰り返す接続は有限回で止め、安定していた接続の過去の失敗は持ち越さない。
const ReconnectSettled = 10 * time.Second

// 再接続の待ち時間は、基準の値にこの範囲の百分率を掛けて揺らす。同時に切れた
// セッションの再接続を散らすためで、設定画面の文言は上限から総所要時間を言う。
const (
	ReconnectJitterMinPercent = 80
	ReconnectJitterMaxPercent = 120
)

// ReconnectBackoff は、n 回目の再接続までに待つ基準の待ち時間。表の末尾以降は最後の
// 値を繰り返す。設定画面の文言はこの表から総所要時間を言う。
var ReconnectBackoff = []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second, 15 * time.Second}

// ReconnectBase は、attempt 回目（0 始まり）の再接続までに待つ、揺らぎを掛ける前の時間を返す。
func ReconnectBase(attempt int) time.Duration {
	return ReconnectBackoff[min(attempt, len(ReconnectBackoff)-1)]
}

// NormaliseReconnects は、範囲の外にある再接続の上限を天井へ戻す。
func NormaliseReconnects(limit int) int {
	if limit < 0 || limit > MaxReconnects {
		return MaxReconnects
	}
	return limit
}

// 自動再接続の problem code。web の画面（web/src/terminal/sessions.ts）が
// 同じ文字列で文言を選ぶ。
const (
	// ProblemReconnectFailed は、再接続が試し直せる理由で失敗したことを表す。
	ProblemReconnectFailed = "reconnect_failed"
	// ProblemReconnectStopped は、利用者が再接続を止めたことを表す。
	ProblemReconnectStopped = "reconnect_stopped"
	// ProblemReconnectExhausted は、再接続の回数の上限に達したことを表す。
	ProblemReconnectExhausted = "reconnect_exhausted"
)

// StopReconnecting abandons the automatic reconnect loop while it is waiting
// or dialing. The pane stays open in the exited state so the user can decide
// later whether to reconnect by hand or close it.
func (s *Session) StopReconnecting() error {
	s.mutex.Lock()
	if s.state != StateReconnecting || s.exited != nil {
		s.mutex.Unlock()
		return ErrNotReconnecting
	}
	s.problem = ProblemReconnectStopped
	handshaking := s.handshakingProcessLocked()
	s.mutex.Unlock()
	s.stopReconnecting()
	if handshaking != nil {
		// reopen は握手前に Process を返す。Ready を待つ側は stopping を見て
		// connected にしないだけで、process 自体は生きて入力を捨て続ける。
		// 閉じて pump に exited まで進ませ、手動の再接続を使える状態にする。
		abandonProcess(handshaking)
	}
	s.publish([]byte("\r\n[sshc] 再接続を停止しました。\r\n"))
	return nil
}

// handshakingProcessLocked は、reopen が返した後で Ready がまだ決まっていない
// process を返す。待機中や dial 中、または確定後は nil を返す。
func (s *Session) handshakingProcessLocked() Process {
	if s.process == nil || s.ready == nil {
		return nil
	}
	select {
	case <-s.ready.done:
		return nil
	default:
		return s.process
	}
}

func (s *Session) stopReconnecting() {
	s.mutex.Lock()
	cancel := s.reconnectCancel
	select {
	case <-s.stopping:
	default:
		close(s.stopping)
	}
	s.mutex.Unlock()
	if cancel != nil {
		cancel()
	}
}

// reconnect は、落ちた輸送を繋ぎ直せたなら真を返す。
func (s *Session) reconnect(info ExitInfo, connectionErr error, now func() time.Time) bool {
	// Dialer.Open は握手前に Process を返す。したがって終了コードが通常の
	// transport loss でなくても、Ready の失敗は接続失敗として扱う。
	if !info.TransportLost && connectionErr == nil {
		return false
	}
	if connectionErr != nil {
		if retry, problem := s.recordReconnectFailure(connectionErr); !retry {
			// 何も書かずに止まると、再接続の途中で止まったのか、もう試さないのかが
			// 画面から分からない。理由の行は接続ログに出ている。
			s.publish([]byte("\r\n[sshc] " + s.reconnectStopNotice(problem) + "\r\n"))
			return false
		}
	}
	for {
		attempt, limit, ok := s.nextReconnectAttempt()
		if !ok || !s.waitToReconnect(attempt, limit, now) {
			return false
		}
		process, stopped, err := s.reopenForReconnect()
		if stopped {
			return false
		}
		if err != nil {
			if !s.recordReopenFailure(err) {
				return false
			}
			continue
		}
		return s.installReconnectedProcess(process)
	}
}

// nextReconnectAttempt は、次の自動再接続を試してよいかと、その回数と上限を返す。
// 上限に達したときは、そのことを problem とターミナルに残す。
func (s *Session) nextReconnectAttempt() (attempt, limit int, ok bool) {
	limit = MaxReconnects
	if s.reconnectLimit != nil {
		limit = NormaliseReconnects(s.reconnectLimit())
	}
	s.mutex.Lock()
	canReopen, attempt := s.reopen != nil, s.reconnectAttempts
	stopped := s.stoppedLocked() || s.exited != nil
	s.mutex.Unlock()
	if canReopen && limit > 0 && attempt >= limit {
		s.mutex.Lock()
		s.problem = ProblemReconnectExhausted
		s.mutex.Unlock()
		s.publish([]byte("\r\n[sshc] 再接続できる回数の上限に達しました。\r\n"))
		return attempt, limit, false
	}
	return attempt, limit, canReopen && !stopped && attempt < limit
}

// waitToReconnect は、再接続までの待ちを一覧とターミナルに出してから待つ。
// 待っているあいだに止められたら false を返す。
func (s *Session) waitToReconnect(attempt, limit int, now func() time.Time) bool {
	wait := ReconnectBase(attempt)
	if s.delay != nil {
		wait = s.delay(attempt)
	}
	retryAt := now().Add(wait)
	s.mutex.Lock()
	s.process = nil
	s.state = StateReconnecting
	s.reconnectView = &ReconnectView{Attempt: attempt + 1, Limit: limit, RetryAt: retryAt}
	s.mutex.Unlock()
	seconds := int((wait + time.Second - 1) / time.Second)
	s.publish([]byte(fmt.Sprintf(
		"\r\n[sshc] SSH接続が切れました。%d秒後に再接続します（%d/%d）。\r\n",
		seconds, attempt+1, limit)))

	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-s.stopping:
		return false
	}
}

// reopenForReconnect は reopen を1回呼ぶ。dial のあいだは StopReconnecting が
// 取り消せるよう、取り消しを reconnectCancel に置く。停止が dial の前に決まって
// いたら dial せず、dial のあいだに決まったら返った process を捨てて、stopped を返す。
func (s *Session) reopenForReconnect() (process Process, stopped bool, err error) {
	attemptCtx, cancel := context.WithCancel(WithAutomaticReconnect(context.Background()))
	s.mutex.Lock()
	if s.stoppedLocked() {
		s.mutex.Unlock()
		cancel()
		return nil, true, nil
	}
	s.reconnectCancel = cancel
	reopen := s.reopen
	// The browser may have been resized during the wait; the new shell
	// must start at that size, so it is read only after waiting.
	size := s.size
	s.mutex.Unlock()
	process, err = reopen(attemptCtx, size)
	s.mutex.Lock()
	s.reconnectCancel = nil
	stopped = s.stoppedLocked()
	s.mutex.Unlock()
	cancel()
	if process != nil && stopped {
		abandonAndWait(process)
		return nil, true, nil
	}
	return process, false, err
}

// installReconnectedProcess は、開き直した process を公開して Ready の観測を始める。
// 公開の直前に停止が決まっていたら、公開せずに捨てて false を返す。
func (s *Session) installReconnectedProcess(process Process) bool {
	s.mutex.Lock()
	stopped := s.stoppedLocked()
	if !stopped {
		s.process = process
		s.reconnectAttempts++
	}
	s.mutex.Unlock()
	if stopped {
		abandonAndWait(process)
		return false
	}
	// Ready が成功するまでは reconnecting のままである。これは新しい
	// shellなので、成功後にだけ前の続きではないことを伝える。
	s.observeProcess(StateReconnecting,
		"\r\n[sshc] 再接続しました。新しいシェルを開始しました。これより前の表示は切断前の記録です。\r\n")
	return true
}

// recordReopenFailure は、開き直せなかった試みを1回と数え、理由をターミナルに書く。
// 試し直さない失敗なら false を返す。
func (s *Session) recordReopenFailure(err error) (retry bool) {
	s.mutex.Lock()
	s.reconnectAttempts++
	s.mutex.Unlock()
	retry, _ = s.recordReconnectFailure(err)
	s.publish([]byte("\r\n[sshc] " + err.Error() + "\r\n"))
	return retry
}

// classifyReconnectFailure は、接続の失敗を、自動で試し直すかと problem code に分ける。
// 分類を持たないセッションと、分類が code を返さなかった失敗は、試し直す
// ProblemReconnectFailed にする。
func (s *Session) classifyReconnectFailure(err error) (retry bool, problem string) {
	if s.reconnectError == nil {
		return true, ProblemReconnectFailed
	}
	retry, problem = s.reconnectError(err)
	if problem == "" {
		problem = ProblemReconnectFailed
	}
	return retry, problem
}

// recordReconnectFailure は、自動再接続の失敗を分類して、待機中の表示に残す。
// 試し直さない失敗は problem にも残す。
func (s *Session) recordReconnectFailure(err error) (retry bool, problem string) {
	retry, problem = s.classifyReconnectFailure(err)
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.reconnectView != nil {
		s.reconnectView.Problem = problem
	}
	if !retry {
		s.problem = problem
	}
	return retry, problem
}

// prepareManualReconnect は終了済みのSSHセッションを同じIDで再利用する。
// 呼び出し側が新しいProcessを確保する間はconnectingとして数え、同時実行と
// session上限の迂回を防ぐ。
func (s *Session) prepareManualReconnect() (func(context.Context, Size) (Process, error), Size, ExitInfo, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.exited == nil || s.reopen == nil || s.discarded {
		return nil, Size{}, ExitInfo{}, ErrReconnectUnavailable
	}
	previous := *s.exited
	s.process = nil
	s.exited = nil
	s.state = StateConnecting
	s.problem = ""
	s.reconnectView = nil
	s.reconnectAttempts = 0
	s.stopping = make(chan struct{})
	s.done = make(chan struct{})
	return s.reopen, s.size, previous, nil
}

// defaultReconnectStopNotice は、再接続を止めた理由に専用の文が無いときに書く文である。
const defaultReconnectStopNotice = "設定を直さない限り同じ理由で失敗するため、自動再接続を停止しました。"

// reconnectStopNotice は、再接続を止めたときにターミナルへ書く文を返す。
func (s *Session) reconnectStopNotice(problem string) string {
	if s.stopNotice != nil {
		if notice := s.stopNotice(problem); notice != "" {
			return notice
		}
	}
	return defaultReconnectStopNotice
}

// failManualReconnect は接続前の終了状態へ戻す。新しく作ったdoneを閉じるため、
// engine停止も失敗した接続を待ち続けない。
func (s *Session) failManualReconnect(previous ExitInfo, problem string) {
	s.mutex.Lock()
	s.process = nil
	s.exited = &previous
	s.state = StateExited
	s.problem = problem
	s.reconnectView = nil
	done := s.done
	s.closeStreamsLocked()
	s.mutex.Unlock()
	close(done)
}

// completeManualReconnect はcloseやshutdownが先行していなければ、新しいProcessを
// 同じsessionへ公開する。
func (s *Session) completeManualReconnect(process Process, started time.Time) bool {
	return s.completeProcessReplacement(process, started,
		"\r\n[sshc] 手動で再接続しました。新しいシェルを開始しました。これより前の表示は切断前の記録です。\r\n")
}

func (s *Session) completeProcessReplacement(process Process, started time.Time, successMessage string) bool {
	s.mutex.Lock()
	if s.stoppedLocked() || s.exited != nil || s.state != StateConnecting {
		s.mutex.Unlock()
		return false
	}
	s.process = process
	s.started = started
	s.problem = ""
	s.reconnectView = nil
	s.mutex.Unlock()
	s.observeProcess(StateConnecting, successMessage)
	return true
}
