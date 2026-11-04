package terminal

import (
	"fmt"
	"time"
)

// networkRetryInterval は、OS が回線について何も言わないマシン（PC）で、回線が戻ったかを確かめる
// 間隔である。ルートの無い接続はパケットを出さずにすぐ失敗するので、短くても負担にならない。これが
// PC の「戻ったらすぐ」の遅れの上限になる。
const networkRetryInterval = 5 * time.Second

// transportCheckLimit は、回線が変わったあと、接続が応答するのを待つ上限である。応答が無ければ
// 切って再接続する。切り替えの直後は遅れることがあり、早く切りすぎるとシェルを失うので長めにする。
const transportCheckLimit = 10 * time.Second

// TransportChecking は、輸送が黙って止まることがある Process である。Wi-Fi とモバイル回線が
// 切り替わると、前の回線の TCP は切れたとも言われないまま応答しなくなり、ターミナルが固まる。
type TransportChecking interface {
	// CheckTransport は、輸送が limit までに応答しなければ閉じ、応答したかを返す。閉じたら Process は
	// 輸送が落ちたとして終わる。
	CheckTransport(limit time.Duration) (answered bool)
}

// watchNetworkChanges は、OS が回線があると言うたびに、つながっている接続が応答するかを確かめる。
// 回線が無いと言われただけでは切らない。すぐ戻れば、同じ TCP のまま続けられる。pump が終わる（done）まで
// 動く。OS が何も言わないマシンでは何もしない。
func (s *Session) watchNetworkChanges(changed <-chan struct{}, done <-chan struct{}) {
	for changed != nil {
		select {
		case <-changed:
		case <-done:
			return
		}
		var available bool
		available, changed = s.network.State()
		if available {
			s.checkTransport()
		}
	}
}

// checkTransport は、つながっている接続が応答するかを確かめる。切れた知らせは確認の結果より先に pump へ
// 届くことがあるので、印は確認を始める前に付け、応答があったときだけ消す。
func (s *Session) checkTransport() {
	s.mutex.Lock()
	checking, canCheck := s.process.(TransportChecking)
	if !canCheck || s.state != StateConnected {
		s.mutex.Unlock()
		return
	}
	s.transportCheckPending = true
	s.mutex.Unlock()
	if checking.CheckTransport(transportCheckLimit) {
		s.mutex.Lock()
		s.transportCheckPending = false
		s.mutex.Unlock()
	}
}

// takeTransportCheckPending は、確認に応答しないまま輸送が落ちたかを返し、印を消す。
func (s *Session) takeTransportCheckPending() bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	pending := s.transportCheckPending
	s.transportCheckPending = false
	return pending
}

// skipWaitAfterNetworkChange は、回線が変わって切れた接続を、待たずに開き直す。waitToReconnect の代わりに
// 呼ぶ。待ち時間は同じ相手へ繰り返し失敗しないためのもので、新しい回線へ移る今回は待つ理由が無い。
// 待ち時間の間に回線の知らせが来ることも無い。知らせはこの切断より前に済んでいる。
func (s *Session) skipWaitAfterNetworkChange(attempt, limit int, now func() time.Time) bool {
	if available, _ := s.network.State(); !available {
		return s.waitForNetwork(attempt, limit, now)
	}
	s.mutex.Lock()
	s.showReconnectingLocked(ReconnectView{Attempt: attempt + 1, Limit: limit, RetryAt: now()})
	s.mutex.Unlock()
	s.publish([]byte("\r\n[sshc] 回線が変わったので、すぐ再接続します。\r\n"))
	return true
}

func (s *Session) lastAttemptHadNoNetwork() bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.noNetwork
}

// waitForNetwork は、回線が戻るまで、再接続の回数を使わずに待つ。止められたら false を返す。
//
// OS が回線の有無を言うマシンでは、回線があると言われるまで待つ。言わないマシンでは、直前の試みが
// 回線の無さで失敗したときに、間隔を置いてまた試す。
func (s *Session) waitForNetwork(attempt, limit int, now func() time.Time) bool {
	s.announceNetworkWait(attempt, limit, now)
	// 回線があると言われているのに直前の試みが回線の無さで失敗したなら、間隔を置いてまた試す。
	// 無いと言われている間は、言葉が変わるまで何も試さない。
	if available, changed := s.network.State(); available {
		timer := time.NewTimer(s.networkRetry)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-changed:
		case <-s.stopping:
			return false
		}
	}
	for {
		available, changed := s.network.State()
		if available {
			return true
		}
		select {
		case <-changed:
		case <-s.stopping:
			return false
		}
	}
}

// announceNetworkWait は、回線を待っていることを一覧に出す。ターミナルに書くのは待ち始めの1回だけである。
// 待つ間の試みは同じ理由で失敗するだけで、書いても行が増えるだけである。
func (s *Session) announceNetworkWait(attempt, limit int, now func() time.Time) {
	s.mutex.Lock()
	s.showReconnectingLocked(ReconnectView{Attempt: attempt + 1, Limit: limit, RetryAt: now(), WaitingForNetwork: true})
	announced := s.networkWaitAnnounced
	s.networkWaitAnnounced = true
	reasonWritten := s.noNetwork
	s.mutex.Unlock()
	if announced {
		return
	}
	lead := ""
	if !reasonWritten {
		lead = reconnectLead(attempt)
	}
	s.publish(fmt.Appendf(nil, "\r\n[sshc] %sネットワークが戻ったらすぐ再接続します。\r\n", lead))
}
