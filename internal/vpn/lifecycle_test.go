package vpn

import (
	"testing"
	"time"
)

// 通っている接続がある経路は、無操作にならない。
func TestARouteWithOpenConnectionsIsNeverIdle(t *testing.T) {
	state := &sessionState{}
	start := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)

	state.borrow()
	state.borrow()

	if idle := state.idleFor(start.Add(time.Hour)); idle != 0 {
		t.Fatalf("接続が2本あるのに無操作と数えた: %v", idle)
	}
	state.release(start)
	if idle := state.idleFor(start.Add(time.Hour)); idle != 0 {
		t.Fatalf("接続が1本残っているのに無操作と数えた: %v", idle)
	}
	state.release(start)
	if idle := state.idleFor(start.Add(30 * time.Minute)); idle != 30*time.Minute {
		t.Fatalf("最後の接続からの長さ = %v", idle)
	}
}

// 二重に閉じられても、数えるのは一度だけである。
//
// 数を間違えると、まだ使われている経路を無操作と見なして畳んでしまう。
func TestClosingOneConnectionTwiceCountsOnce(t *testing.T) {
	state := &sessionState{}
	start := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	state.borrow()
	state.borrow()

	closed := &countedConnection{release: func() { state.release(start) }}
	closed.once.Do(closed.release)
	closed.once.Do(closed.release)

	if idle := state.idleFor(start.Add(time.Hour)); idle != 0 {
		t.Fatalf("1本閉じただけで無操作になった: %v", idle)
	}
}

// 一度も使われていない状態は、無操作とは呼ばない。
//
// 起こしたばかりの経路を、次の接続が来る前に畳まないためである。
func TestARouteThatWasNeverUsedIsNotReportedAsIdle(t *testing.T) {
	state := &sessionState{}

	if idle := state.idleFor(time.Now()); idle != 0 {
		t.Fatalf("使われる前の経路を無操作と数えた: %v", idle)
	}
}

// 起動が長引いているあいだでも、いまどこまで進んだかは読める。
//
// 段階を state.mutex で守ると、起動が終わるまで状態を読む側が待たされる。
// 何分かかるか分からない相手を待っているときに、何も答えられなくなる。
func TestThePhaseIsReadableWhileAStartHoldsTheSessionLock(t *testing.T) {
	state := &sessionState{}
	state.mutex.Lock()
	defer state.mutex.Unlock()

	state.enterPhase(PhaseImage)
	if phase := state.currentPhase(); phase != PhaseImage {
		t.Fatalf("phase = %q", phase)
	}
	state.enterPhase(PhaseTunnel)
	if phase := state.currentPhase(); phase != PhaseTunnel {
		t.Fatalf("phase = %q", phase)
	}
}

// 用意していない経路は、どの段階にもいない。
func TestARouteThatIsNotBeingOpenedReportsNoPhase(t *testing.T) {
	state := &sessionState{}

	if phase := state.currentPhase(); phase != "" {
		t.Fatalf("phase = %q", phase)
	}
	state.enterPhase(PhaseContainer)
	state.enterPhase("")
	if phase := state.currentPhase(); phase != "" {
		t.Fatalf("起動が終わったあとに段階が残った: %q", phase)
	}
}
