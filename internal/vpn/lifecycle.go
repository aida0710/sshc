package vpn

import (
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// 経路ひとつぶんの寿命を持つ。どこまで用意できたか、いま何本通っているか、
// 最後の一本が終わってからどれだけ経ったか。Manager はこれを名前で引く。

// sessionState は、プロファイルひとつぶんの進行中の状態である。
type sessionState struct {
	// transition は、起動と停止を1本にする。起動は分単位になりうるので、
	// 状態を読むだけの側はこの鍵を使わない。
	transition sync.Mutex

	// use は、下の値を守る。どれも読み書きが一瞬で終わる。
	use sync.Mutex
	// started は、いまコンテナが提供している設定である。設定が変わったら
	// 作り直す。古い設定のまま繋ぎ続けると、利用者が直した先へ行かない。
	started Profile
	running bool
	// open は、この経路を通っている接続と、これから通る予約の数である。
	// 予約を数えないと、起動が終わってから接続が数えられるまでの隙間に、
	// 無操作として畳まれうる。
	open      int
	idleSince time.Time
	// relay は、engine が差し出す中継の待ち受けである。経路が無ければ nil。
	relay *engineRelay
	// failureLogs は、最後に用意できなかったときのコンテナのログである（秘密は
	// 伏せてある）。そのコンテナはもう無いので、ここにしか残っていない。
	failureLogs string

	// phase は、経路を用意しているあいだの段階である。起動中は transition が
	// 握られたままなので、段階は鍵を使わずに読み書きする。
	phase atomic.Value
}

// StartPhase は、経路がどこまでできたかである。空なら用意していない。
type StartPhase string

const (
	// PhaseImage は、コンテナのイメージを用意しているところである。初回は
	// 取得と構築に分単位でかかる。
	PhaseImage StartPhase = "image"
	// PhaseContainer は、コンテナを起こして設定を渡しているところである。
	PhaseContainer StartPhase = "container"
	// PhaseTunnel は、トンネルが上がって中継が立つのを待っているところである。
	PhaseTunnel StartPhase = "tunnel"
	// PhaseApproval は、利用者が電話で承認するのを待っているところである。
	// 待っているのが機械ではなく人なので、トンネル待ちとは別に見せる。
	PhaseApproval StartPhase = "approval"
)

// enterPhase は、いまの段階を記録する。空文字列は用意していないことを表す。
func (state *sessionState) enterPhase(phase StartPhase) {
	state.phase.Store(phase)
}

// currentPhase は、いまの段階を返す。
func (state *sessionState) currentPhase() StartPhase {
	stored, _ := state.phase.Load().(StartPhase)
	return stored
}

// isRunning は、この engine がこの経路を起こしたままかを返す。
func (state *sessionState) isRunning() bool {
	state.use.Lock()
	defer state.use.Unlock()
	return state.running
}

// serves は、この経路が profile の設定のまま動いていて、中継を差し出しているかを返す。
func (state *sessionState) serves(profile Profile) bool {
	state.use.Lock()
	defer state.use.Unlock()
	return state.running && state.relay != nil && state.started.sameRouteAs(profile)
}

// markStarted は、経路が用意できたことを記録する。無操作の長さはここから数える。
//
// 起動しただけで一度も使われない経路（`vpn up` など）も、ほかと同じ長さで畳む。
func (state *sessionState) markStarted(profile Profile, relay *engineRelay, now time.Time) {
	state.use.Lock()
	defer state.use.Unlock()
	state.started, state.running, state.relay = profile, true, relay
	state.idleSince = now
}

// markStopped は、経路が無くなったことを記録し、差し出していた中継を返す。
// 呼び出し側がそれを閉じる。
func (state *sessionState) markStopped() *engineRelay {
	state.use.Lock()
	defer state.use.Unlock()
	relay := state.relay
	state.running, state.relay = false, nil
	return relay
}

// relaySocket は、engine が差し出している中継の場所を返す。無ければ空。
func (state *sessionState) relaySocket() string {
	state.use.Lock()
	defer state.use.Unlock()
	if state.relay == nil {
		return ""
	}
	return state.relay.path
}

// keepFailureLogs は、用意できなかったコンテナのログを覚えておく。
func (state *sessionState) keepFailureLogs(logs string) {
	state.use.Lock()
	defer state.use.Unlock()
	state.failureLogs = logs
}

// lastFailureLogs は、最後に用意できなかったときのログを返す。
func (state *sessionState) lastFailureLogs() string {
	state.use.Lock()
	defer state.use.Unlock()
	return state.failureLogs
}

// borrow は、この経路を通る接続（または予約）がひとつ増えたことを記録する。
func (state *sessionState) borrow() {
	state.use.Lock()
	defer state.use.Unlock()
	state.open++
}

// release は、接続がひとつ終わったことを記録する。最後の一本が終わった時刻を
// 覚えておき、無操作の長さを測れるようにする。
func (state *sessionState) release(now time.Time) {
	state.use.Lock()
	defer state.use.Unlock()
	if state.open > 0 {
		state.open--
	}
	if state.open == 0 {
		state.idleSince = now
	}
}

// idleFor は、接続が一本も無い状態が続いている長さを返す。一本でも通っていれば
// 0 を返す。まだ一度も用意していない経路も 0 を返す。
func (state *sessionState) idleFor(now time.Time) time.Duration {
	state.use.Lock()
	defer state.use.Unlock()
	if state.open > 0 || state.idleSince.IsZero() {
		return 0
	}
	return now.Sub(state.idleSince)
}

// idleLongerThan は、動いていて、用意の途中でもなく、idle より長く誰も通って
// いない経路かを返す。
func (state *sessionState) idleLongerThan(now time.Time, idle time.Duration) bool {
	return state.currentPhase() == "" && state.isRunning() && state.idleFor(now) >= idle
}

// countedConnection は、閉じられたことを数える接続である。
//
// 二重に Close されても一度しか数えない。数を間違えると、使っている経路を
// 無操作と見なして畳んでしまう。
type countedConnection struct {
	net.Conn
	once    sync.Once
	release func()
}

func (connection *countedConnection) Close() error {
	connection.once.Do(connection.release)
	return connection.Conn.Close()
}
