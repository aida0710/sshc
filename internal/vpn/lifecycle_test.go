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
// 段階を state.transition で守ると、起動が終わるまで状態を読む側が待たされる。
// 何分かかるか分からない相手を待っているときに、何も答えられなくなる。
func TestThePhaseIsReadableWhileAStartHoldsTheSessionLock(t *testing.T) {
	state := &sessionState{}
	state.transition.Lock()
	defer state.transition.Unlock()

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

// 起動しただけで一度も使われない経路も、起動した時刻から無操作を数える。
//
// 数えないと、`vpn up` だけの経路が engine の寿命のあいだ残り続ける。
func TestARouteThatWasOnlyStartedCountsIdleFromItsStart(t *testing.T) {
	state := &sessionState{}
	start := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)

	state.markStarted(Profile{Name: "lab"}, nil, start)

	if !state.idleLongerThan(start.Add(11*time.Minute), 10*time.Minute) {
		t.Fatal("起動から10分を過ぎた経路を無操作と数えなかった")
	}
}

// 前に使い終えた時刻が古くても、作り直した経路は作り直した時刻から数える。
func TestARestartedRouteForgetsTheIdleTimeOfItsPreviousUse(t *testing.T) {
	state := &sessionState{}
	long := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	state.borrow()
	state.release(long)

	restarted := long.Add(time.Hour)
	state.markStarted(Profile{Name: "lab"}, nil, restarted)

	if state.idleLongerThan(restarted.Add(time.Minute), 10*time.Minute) {
		t.Fatal("作り直した直後の経路を無操作と数えた")
	}
}

// 起動を待っている接続（予約）がある経路は、無操作にならない。
func TestAReservationKeepsTheRouteFromBeingIdle(t *testing.T) {
	state := &sessionState{}
	start := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	state.markStarted(Profile{Name: "lab"}, nil, start)

	state.borrow()

	if state.idleLongerThan(start.Add(time.Hour), 10*time.Minute) {
		t.Fatal("起動を待つ接続があるのに無操作と数えた")
	}
}

// 用意の途中の経路は、無操作として畳まない。
func TestARouteBeingPreparedIsNotIdle(t *testing.T) {
	state := &sessionState{}
	start := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	state.markStarted(Profile{Name: "lab"}, nil, start)

	state.enterPhase(PhaseTunnel)

	if state.idleLongerThan(start.Add(time.Hour), 10*time.Minute) {
		t.Fatal("用意の途中の経路を無操作と数えた")
	}
}

// 起動を求められた経路は、無操作の起点をいまにする。
//
// CLI は起動を求めてから engine の中継へ繋ぐ。そのあいだに、前回の接続から数えた
// 無操作で停止されないためである。
func TestAskingForARunningRouteRestartsItsIdleClock(t *testing.T) {
	state := &sessionState{}
	start := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	state.borrow()
	state.release(start)

	state.touch(start.Add(time.Hour))

	if idle := state.idleFor(start.Add(time.Hour + time.Minute)); idle != time.Minute {
		t.Fatalf("idle = %v", idle)
	}
}

// 通っている接続がある経路の起点は動かさない。
func TestTouchingARouteInUseChangesNothing(t *testing.T) {
	state := &sessionState{}
	start := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	state.borrow()

	state.touch(start)

	if idle := state.idleFor(start.Add(time.Hour)); idle != 0 {
		t.Fatalf("idle = %v", idle)
	}
}

// 知らせる文は、時間のかかる段階と、利用者が何かをする段階だけが持つ。
func TestOnlyLongOrHumanPhasesHaveANotice(t *testing.T) {
	for phase, wanted := range map[StartPhase]bool{
		PhaseImage: true, PhaseApproval: true, PhaseContainer: false, PhaseTunnel: false, "": false,
	} {
		if got := phase.Notice() != ""; got != wanted {
			t.Errorf("%q.Notice() = %q", phase, phase.Notice())
		}
	}
}
