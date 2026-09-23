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
	mutex sync.Mutex
	// started は、いまコンテナが提供している設定である。設定が変わったら
	// 作り直す。古い設定のまま繋ぎ続けると、利用者が直した先へ行かない。
	started Profile
	running bool

	// use は、この経路を通っている接続の数を数える。起動や停止は時間の
	// かかる操作なので、数えるのは別の鍵で守る。数えるだけの Close が、
	// 進行中の起動を待つ理由はない。
	use       sync.Mutex
	open      int
	idleSince time.Time

	// phase は、経路を用意しているあいだの段階である。起動は分単位になる
	// ことがあり、その最中に状態を読む側を待たせたくない。mutex は起動が
	// 終わるまで握られたままなので、段階は鍵を使わずに読み書きする。
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
	state.mutex.Lock()
	defer state.mutex.Unlock()
	return state.running
}

// borrow は、この経路を通る接続がひとつ増えたことを記録する。
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
// 0 を返す。
func (state *sessionState) idleFor(now time.Time) time.Duration {
	state.use.Lock()
	defer state.use.Unlock()
	if state.open > 0 || state.idleSince.IsZero() {
		return 0
	}
	return now.Sub(state.idleSince)
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
