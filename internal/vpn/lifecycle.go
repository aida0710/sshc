package vpn

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// 経路ひとつぶんの寿命を持つ。どこまで用意できたか、いま何本通っているか、
// 最後の一本が終わってからどれだけ経ったか。Manager はこれを名前で引く。

// routeState は、プロファイルひとつぶんの経路の状態である。
type routeState struct {
	// transition は、起動と停止を1本にする。起動は分単位になりうるので、
	// 状態を読むだけの側はこの鍵を使わない。待つ側は ctx が終われば待つのをやめる。
	transition contextLock

	// use は、下の値を守る。どれも読み書きが一瞬で終わる。
	use sync.Mutex
	// started は、いまコンテナが提供している経路である。設定かシークレットが
	// 変わったら作り直す。古いまま繋ぎ続けると、利用者が直した先へ行かない。
	started routeIdentity
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
	// disconnected は、利用者がこの経路を切断し、まだ起動し直していないことを表す。
	// 切断で切れた接続の自動再接続が、経路を起動し直さないようにするためである。
	disconnected bool
	// changedAt は、sshcエンジンがこの経路を最後に起動・停止したときの変更の番号
	// である（Manager.changes）。これより前に読み始めた docker の状態より新しい。
	changedAt uint64

	// phase は、経路を用意しているあいだの段階である。起動中は transition が
	// 握られたままなので、段階は鍵を使わずに読み書きする。
	phase atomic.Value

	// cancelStart は、transition を握って進んでいる起動を、理由を添えて打ち切る。起動して
	// いなければ nil。停止を頼まれたら、起動が終わるのを待たずに打ち切る。
	cancelStart context.CancelCauseFunc
	// stops は、停止・切断・削除を頼まれた回数である。鍵を待っている起動には cancelStart が
	// 届かないので、起動は鍵を待つ前にこの回数を控え、鍵を取ったときに増えていれば起動しない。
	// 停止より先に鍵を待ち始めた起動が、停止のあとで経路を起動し直さないためである。
	stops uint64
	// stopCause は、最後に頼まれた停止が起動を打ち切る理由である（ErrRouteDisconnected か
	// ErrRouteStopped）。
	stopCause error

	// record は、この経路について sshcエンジンが行ったことの記録である。
	record attemptRecord
}

// StartPhase は、経路がどこまでできたかである。空なら用意していない。
type StartPhase string

const (
	// PhaseImage は、コンテナのイメージを作っているところである。初回は取得と
	// 構築に分単位でかかる。イメージが作成済みなら、この段階は通らない。
	PhaseImage StartPhase = "image"
	// PhaseContainer は、コンテナを起こして設定を渡しているところである。
	PhaseContainer StartPhase = "container"
	// PhaseTunnel は、トンネルが上がって中継が立つのを待っているところである。
	PhaseTunnel StartPhase = "tunnel"
	// PhaseApproval は、利用者が電話で承認するのを待っているところである。
	// 待っているのが機械ではなく人なので、トンネル待ちとは別に見せる。
	PhaseApproval StartPhase = "approval"
)

// Notice は、この段階に入ったことを、接続ログの設定に関係なく知らせる文である。
// 時間がかかる段階と、利用者が何かをする段階だけが文を持ち、それ以外は空を返す。
//
// Terminal の接続は sshcエンジンの中でこの文を書き、CLI の接続は sshcエンジンの
// 段階を見て同じ文を書く。
func (phase StartPhase) Notice() string {
	switch phase {
	case PhaseImage:
		return "VPNのコンテナイメージを作成しています。初回は数分かかることがあります。"
	case PhaseApproval:
		return "スマートフォンでの承認を待っています。"
	default:
		return ""
	}
}

// enterPhase は、いまの段階を記録する。空文字列は用意していないことを表す。
func (state *routeState) enterPhase(phase StartPhase) {
	state.phase.Store(phase)
}

// currentPhase は、いまの段階を返す。
func (state *routeState) currentPhase() StartPhase {
	stored, _ := state.phase.Load().(StartPhase)
	return stored
}

// isRunning は、この engine がこの経路を起こしたままかを返す。
func (state *routeState) isRunning() bool {
	state.use.Lock()
	defer state.use.Unlock()
	return state.running
}

// serves は、この経路が route の設定とシークレットのまま動いていて、中継を差し出しているかを
// 返す。
func (state *routeState) serves(route routeIdentity) bool {
	state.use.Lock()
	defer state.use.Unlock()
	return state.running && state.relay != nil && state.started.same(route)
}

// markStarted は、経路が用意できたことを記録する。無操作の長さはここから数える。
//
// 起動しただけで一度も使われない経路（`vpn up` など）も、ほかと同じ長さで畳む。
func (state *routeState) markStarted(route routeIdentity, relay *engineRelay, now time.Time) {
	state.use.Lock()
	defer state.use.Unlock()
	state.started, state.running, state.relay = route, true, relay
	state.idleSince = now
	// 前回用意できなかったときのログは、もういまの経路のものではない。
	state.failureLogs = ""
}

// touch は、接続が一本も通っていなければ、無操作の起点を now にする。
func (state *routeState) touch(now time.Time) {
	state.use.Lock()
	defer state.use.Unlock()
	if state.open == 0 {
		state.idleSince = now
	}
}

// markStopped は、経路が無くなったことを記録し、差し出していた中継を返す。
// 呼び出し側がそれを閉じる。
func (state *routeState) markStopped() *engineRelay {
	state.use.Lock()
	defer state.use.Unlock()
	relay := state.relay
	state.running, state.relay = false, nil
	return relay
}

// relaySocket は、engine が差し出している中継の場所を返す。無ければ空。
func (state *routeState) relaySocket() string {
	state.use.Lock()
	defer state.use.Unlock()
	if state.relay == nil {
		return ""
	}
	return state.relay.path
}

// stopsRequested は、これまでに頼まれた停止の回数を返す。起動は鍵を待つ前に控える。
func (state *routeState) stopsRequested() uint64 {
	state.use.Lock()
	defer state.use.Unlock()
	return state.stops
}

// beginStart は、鍵を取った起動を cancel で打ち切れるようにする。控えた回数 stops のあとに
// 停止を頼まれていれば、登録せずにその停止の理由を返す。確かめるのと登録するのを1回で
// 行うので、そのあいだに頼まれた停止も取りこぼさない。transition を握って呼ぶ。
func (state *routeState) beginStart(stops uint64, cancel context.CancelCauseFunc) error {
	state.use.Lock()
	defer state.use.Unlock()
	if state.stops != stops {
		return state.stopCause
	}
	state.cancelStart = cancel
	return nil
}

// endStart は、起動が終わったことを記録する。transition を握って呼ぶ。
func (state *routeState) endStart() {
	state.use.Lock()
	defer state.use.Unlock()
	state.cancelStart = nil
}

// requestStop は、停止を頼まれたことを記録し、進んでいる起動があれば cause で打ち切る。
// 停止は鍵を待つ前に呼ぶ。起動は片付けてから transition を返す。
func (state *routeState) requestStop(cause error) {
	state.use.Lock()
	defer state.use.Unlock()
	state.stops++
	state.stopCause = cause
	if state.cancelStart != nil {
		state.cancelStart(cause)
	}
}

// stopCauseOf は、ctx が停止・切断・削除で打ち切られていれば、その理由を返す。そうで
// なければ nil を返す。打ち切られた起動は失敗ではないので、失敗として記録しない。
func stopCauseOf(ctx context.Context) error {
	cause := context.Cause(ctx)
	if errors.Is(cause, ErrRouteDisconnected) || errors.Is(cause, ErrRouteStopped) {
		return cause
	}
	return nil
}

// stopCauseOr は、ctx が停止・切断・削除で打ち切られていれば、その理由を返す。そうで
// なければ err を返す。停止で打ち切った起動を、どの段で打ち切られても停止の理由で終える
// ために使う。イメージの作成の失敗や素の取り消しとして返すと、起動を頼んだほかの入口
// では故障に見える。
func stopCauseOr(ctx context.Context, err error) error {
	if cause := stopCauseOf(ctx); cause != nil {
		return cause
	}
	return err
}

// forgetHistory は、この名前の経路について覚えている記録と、用意できなかったときの
// ログを捨てる。削除や改名のあとで同じ名前のプロファイルを作ったときに、前の
// プロファイルの記録を見せない。
func (state *routeState) forgetHistory() {
	state.use.Lock()
	state.failureLogs = ""
	state.use.Unlock()
	state.record.forget()
}

// keepFailureLogs は、用意できなかったコンテナのログを覚えておく。
func (state *routeState) keepFailureLogs(logs string) {
	state.use.Lock()
	defer state.use.Unlock()
	state.failureLogs = logs
}

// lastFailureLogs は、最後に用意できなかったときのログを返す。
func (state *routeState) lastFailureLogs() string {
	state.use.Lock()
	defer state.use.Unlock()
	return state.failureLogs
}

// borrow は、この経路を通る接続（または予約）がひとつ増えたことを記録する。
func (state *routeState) borrow() {
	state.use.Lock()
	defer state.use.Unlock()
	state.open++
}

// release は、接続がひとつ終わったことを記録する。最後の一本が終わった時刻を
// 覚えておき、無操作の長さを測れるようにする。
func (state *routeState) release(now time.Time) {
	state.use.Lock()
	defer state.use.Unlock()
	if state.open > 0 {
		state.open--
	}
	if state.open == 0 {
		state.idleSince = now
	}
}

// setDisconnected は、利用者がこの経路を切断したままかどうかを記録する。
func (state *routeState) setDisconnected(disconnected bool) {
	state.use.Lock()
	defer state.use.Unlock()
	state.disconnected = disconnected
}

// isDisconnected は、利用者がこの経路を切断し、まだ起動し直していないかを返す。
func (state *routeState) isDisconnected() bool {
	state.use.Lock()
	defer state.use.Unlock()
	return state.disconnected
}

// noteChanged は、sshcエンジンがこの経路を起動・停止したときの変更の番号を記録する。
func (state *routeState) noteChanged(change uint64) {
	state.use.Lock()
	defer state.use.Unlock()
	state.changedAt = change
}

// changedSince は、変更の番号が since より後に、sshcエンジンがこの経路を起動・停止
// したかを返す。
func (state *routeState) changedSince(since uint64) bool {
	state.use.Lock()
	defer state.use.Unlock()
	return state.changedAt > since
}

// openConnections は、この経路を通っている接続と予約の数を返す。
func (state *routeState) openConnections() int {
	state.use.Lock()
	defer state.use.Unlock()
	return state.open
}

// idleFor は、接続が一本も無い状態が続いている長さを返す。一本でも通っていれば
// 0 を返す。まだ一度も用意していない経路も 0 を返す。
func (state *routeState) idleFor(now time.Time) time.Duration {
	state.use.Lock()
	defer state.use.Unlock()
	if state.open > 0 || state.idleSince.IsZero() {
		return 0
	}
	return now.Sub(state.idleSince)
}

// idleLongerThan は、動いていて、用意の途中でもなく、idle より長く誰も通って
// いない経路かを返す。
func (state *routeState) idleLongerThan(now time.Time, idle time.Duration) bool {
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
