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
	// CheckTransport は、輸送が limit までに応答しなければ閉じる。閉じたら Process は輸送が
	// 落ちたとして終わる。
	CheckTransport(limit time.Duration)
}

// watchNetworkChanges は、OS が回線があると言うたびに、つながっている接続が応答するかを確かめる。
// 回線が無いと言われただけでは切らない。すぐ戻れば、同じ TCP のまま続けられる。pump が終わる（done）まで
// 動く。OS が何も言わないマシンでは何もしない。
func (s *Session) watchNetworkChanges(done <-chan struct{}) {
	_, changed := s.network.State()
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

func (s *Session) checkTransport() {
	s.mutex.Lock()
	process, state := s.process, s.state
	s.mutex.Unlock()
	if checking, ok := process.(TransportChecking); ok && state == StateConnected {
		checking.CheckTransport(transportCheckLimit)
	}
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
	s.process = nil
	s.state = StateReconnecting
	s.reconnectView = &ReconnectView{Attempt: attempt + 1, Limit: limit, RetryAt: now(), WaitingForNetwork: true}
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
