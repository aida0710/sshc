package terminal_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"sshc/internal/terminal"
	"sshc/internal/testwait"
)

// noticeNetworkChange は、回線が変わって切れた接続を繋ぎ直すときにターミナルへ書く行である。
const noticeNetworkChange = "回線が変わったので、すぐ再接続します"

// noticeTimedRetry は、待ち時間を置いて繋ぎ直すときにターミナルへ書く行である。
const noticeTimedRetry = "秒後に再接続します"

// neverDuringTheTest は、待ち時間が飛ばされたことを見分けるための待ち時間である。
const neverDuringTheTest = time.Hour

// answerSettleTime は、確認の答えを受けたセッションが印を消し終えるまでに見込む時間である。
const answerSettleTime = 100 * time.Millisecond

// checkableProcess は、回線が変わったあとの確認に何と答えるかを試験が決める接続である。
type checkableProcess struct {
	*readyProcess
	asked   chan struct{}
	answers chan bool
}

func (p *checkableProcess) CheckTransport(time.Duration) bool {
	p.asked <- struct{}{}
	return <-p.answers
}

// waitForCheck は、確認が始まるまで待つ。
func (p *checkableProcess) waitForCheck(t *testing.T) {
	t.Helper()
	select {
	case <-p.asked:
	case <-time.After(5 * time.Second):
		t.Fatal("the connection was not checked")
	}
}

type checkableOpenSpy struct {
	mutex     sync.Mutex
	processes []*checkableProcess
}

func (s *checkableOpenSpy) open(context.Context, terminal.Size) (terminal.Process, error) {
	process := &checkableProcess{
		readyProcess: newReadyProcess(),
		asked:        make(chan struct{}, 1),
		answers:      make(chan bool, 1),
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.processes = append(s.processes, process)
	return process, nil
}

func (s *checkableOpenSpy) at(index int) *checkableProcess {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.processes[index]
}

func (s *checkableOpenSpy) count() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return len(s.processes)
}

// openCheckable は、回線の知らせを受けるレジストリでセッションを開き、最初の接続を完了させる。
func openCheckable(t *testing.T, registry *terminal.Registry, spy *checkableOpenSpy) *terminal.Session {
	t.Helper()
	registry.Network = terminal.NewNetworkWatch()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}
	connect(t, session, spy, 0)
	return session
}

// connect は、index 番目の接続の試みを成功させ、つながるまで待つ。
func connect(t *testing.T, session *terminal.Session, spy *checkableOpenSpy, index int) {
	t.Helper()
	testwait.Until(t, func() bool { return spy.count() > index })
	spy.at(index).finishOpen(nil)
	testwait.Until(t, func() bool { return session.View().State == terminal.StateConnected })
}

// loseToNetworkChange は、回線が変わったあとの確認に index 番目の接続が応答しないまま切れた状態にする。
func loseToNetworkChange(t *testing.T, registry *terminal.Registry, spy *checkableOpenSpy, index int) {
	t.Helper()
	process := spy.at(index)
	registry.Network.SetAvailable(true)
	process.waitForCheck(t)
	process.exit(transportLost)
	t.Cleanup(func() { process.answers <- false })
}

func TestAConnectionLostWhileItsCheckIsUnansweredReconnectsWithoutWaiting(t *testing.T) {
	spy := &checkableOpenSpy{}
	registry, _ := newFastRegistry()
	registry.ReconnectDelay = func(int) time.Duration { return neverDuringTheTest }
	session := openCheckable(t, registry, spy)

	loseToNetworkChange(t, registry, spy, 0)

	connect(t, session, spy, 1)
	output := string(snapshotOf(session))
	if !strings.Contains(output, noticeNetworkChange) {
		t.Errorf("the terminal does not say the network changed: %q", output)
	}
	if strings.Contains(output, noticeTimedRetry) {
		t.Errorf("the terminal announced a wait that was not taken: %q", output)
	}
}

// 機内モードから戻ると、モバイル回線で繋ぎ直した数秒後に Wi-Fi へ替わって切れる。上限が1回でも繋ぎ直す。
func TestALimitOfOneAttemptStillReconnectsAfterTheNetworkChanges(t *testing.T) {
	spy := &checkableOpenSpy{}
	registry, _ := newFastRegistry()
	registry.ReconnectLimit = func() int { return 1 }
	session := openCheckable(t, registry, spy)
	spy.at(0).exit(transportLost)
	connect(t, session, spy, 1)

	loseToNetworkChange(t, registry, spy, 1)

	connect(t, session, spy, 2)
	if output := string(snapshotOf(session)); strings.Contains(output, "再接続できる回数の上限に達しました") {
		t.Errorf("the limit stopped the reconnect: %q", output)
	}
}

func TestAFailedImmediateReconnectFallsBackToTheFirstRetryWait(t *testing.T) {
	spy := &checkableOpenSpy{}
	registry, _ := newFastRegistry()
	var mutex sync.Mutex
	var waitedAttempts []int
	registry.ReconnectDelay = func(attempt int) time.Duration {
		mutex.Lock()
		defer mutex.Unlock()
		waitedAttempts = append(waitedAttempts, attempt)
		return 0
	}
	session := openCheckable(t, registry, spy)

	loseToNetworkChange(t, registry, spy, 0)
	testwait.Until(t, func() bool { return spy.count() == 2 })
	spy.at(1).finishOpen(errors.New("connection refused"))
	spy.at(1).exit(terminal.ExitInfo{Code: 255})

	connect(t, session, spy, 2)
	mutex.Lock()
	defer mutex.Unlock()
	if !slices.Equal(waitedAttempts, []int{1}) {
		t.Errorf("waited before attempts %v, want only before the attempt after the failed one", waitedAttempts)
	}
}

func TestAConnectionThatAnsweredItsCheckWaitsAsUsualWhenItIsLostLater(t *testing.T) {
	spy := &checkableOpenSpy{}
	registry, _ := newFastRegistry()
	registry.ReconnectDelay = func(int) time.Duration { return neverDuringTheTest }
	session := openCheckable(t, registry, spy)
	process := spy.at(0)
	registry.Network.SetAvailable(true)
	process.waitForCheck(t)
	process.answers <- true
	time.Sleep(answerSettleTime)

	process.exit(transportLost)

	testwait.Until(t, func() bool { return strings.Contains(string(snapshotOf(session)), noticeTimedRetry) })
	if output := string(snapshotOf(session)); strings.Contains(output, noticeNetworkChange) {
		t.Errorf("a later loss was blamed on the network change: %q", output)
	}
	if spy.count() != 1 {
		t.Errorf("reconnected without waiting: %d opens", spy.count())
	}
	if err := session.StopReconnecting(); err != nil {
		t.Fatal(err)
	}
}

func TestAConnectionLostToANetworkChangeWaitsForTheNetworkWhenItIsGoneAgain(t *testing.T) {
	spy := &checkableOpenSpy{}
	registry, _ := newFastRegistry()
	session := openCheckable(t, registry, spy)
	process := spy.at(0)
	registry.Network.SetAvailable(true)
	process.waitForCheck(t)
	t.Cleanup(func() { process.answers <- false })

	registry.Network.SetAvailable(false)
	process.exit(transportLost)

	testwait.Until(t, func() bool {
		reconnect := session.View().Reconnect
		return reconnect != nil && reconnect.WaitingForNetwork
	})
	if spy.count() != 1 {
		t.Fatalf("tried with no network: %d opens", spy.count())
	}
	registry.Network.SetAvailable(true)
	connect(t, session, spy, 1)
}

func TestAutomaticReconnectTurnedOffStaysOffAfterANetworkChange(t *testing.T) {
	spy := &checkableOpenSpy{}
	registry, _ := newFastRegistry()
	registry.ReconnectLimit = func() int { return 0 }
	session := openCheckable(t, registry, spy)

	loseToNetworkChange(t, registry, spy, 0)

	testwait.Until(t, func() bool { return !session.Live() })
	if spy.count() != 1 {
		t.Errorf("reconnected with automatic reconnect turned off: %d opens", spy.count())
	}
}
