package terminal_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"sshc/internal/terminal"
	"sshc/internal/testwait"
)

var errNoRoute = errors.New("connect: network is unreachable")

// classifyNoNetwork は、HTTP の層の reconnectError と同じ形で、回線の無さを見分ける。
func classifyNoNetwork(err error) (bool, string) {
	if errors.Is(err, errNoRoute) {
		return true, terminal.ProblemNoNetwork
	}
	return true, terminal.ProblemReconnectFailed
}

func openReady(t *testing.T, registry *terminal.Registry, spy *readyOpenSpy) *terminal.Session {
	t.Helper()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway",
		Open: spy.open, ReconnectError: classifyNoNetwork,
	})
	if err != nil {
		t.Fatal(err)
	}
	spy.at(0).finishOpen(nil)
	testwait.Until(t, func() bool { return session.View().State == terminal.StateConnected })
	return session
}

// failAttempt は、index 番目の接続の試みを err で失敗させる。
func failAttempt(t *testing.T, spy *readyOpenSpy, index int, err error) {
	t.Helper()
	testwait.Until(t, func() bool { return spy.count() > index })
	spy.at(index).finishOpen(err)
	spy.at(index).exit(terminal.ExitInfo{Code: 255})
}

func TestWhileTheOperatingSystemSaysThereIsNoNetworkNoAttemptIsMadeOrCounted(t *testing.T) {
	spy := &readyOpenSpy{}
	registry, _ := newFastRegistry()
	registry.Network = terminal.NewNetworkWatch()
	session := openReady(t, registry, spy)

	registry.Network.SetAvailable(false)
	spy.at(0).exit(transportLost)
	testwait.Until(t, func() bool {
		view := session.View()
		return view.State == terminal.StateReconnecting && view.Reconnect != nil && view.Reconnect.WaitingForNetwork
	})
	time.Sleep(20 * time.Millisecond)
	if spy.count() != 1 {
		t.Fatalf("%d connections opened with no network, want only the first", spy.count())
	}
	if view := session.View(); view.Reconnect.Attempt != 1 {
		t.Errorf("waiting used up an attempt: %+v", view.Reconnect)
	}
	if !strings.Contains(string(snapshotOf(session)), "ネットワークが戻ったらすぐ再接続します") {
		t.Errorf("the terminal does not say it waits: %q", snapshotOf(session))
	}

	registry.Network.SetAvailable(true)
	testwait.Until(t, func() bool { return spy.count() == 2 })
	spy.at(1).finishOpen(nil)
	testwait.Until(t, func() bool { return session.View().State == terminal.StateConnected })
}

// 回線が戻るのは、待ち時間の途中でも同じである。
func TestANetworkThatComesBackEndsTheWaitForTheNextAttempt(t *testing.T) {
	spy := &readyOpenSpy{}
	registry, _ := newFastRegistry()
	registry.ReconnectDelay = func(int) time.Duration { return time.Hour }
	registry.Network = terminal.NewNetworkWatch()
	session := openReady(t, registry, spy)

	spy.at(0).exit(transportLost)
	testwait.Until(t, func() bool { return session.View().Reconnect != nil })
	if spy.count() != 1 {
		t.Fatalf("%d connections opened during the wait", spy.count())
	}
	registry.Network.SetAvailable(true)
	testwait.Until(t, func() bool { return spy.count() == 2 })
	spy.at(1).finishOpen(nil)
	testwait.Until(t, func() bool { return session.View().State == terminal.StateConnected })
}

func TestAnAttemptThatFindsNoNetworkIsNotCountedAndIsTriedAgain(t *testing.T) {
	spy := &readyOpenSpy{}
	registry, _ := newFastRegistry()
	registry.ReconnectLimit = func() int { return 1 }
	session := openReady(t, registry, spy)

	spy.at(0).exit(transportLost)
	failAttempt(t, spy, 1, errNoRoute)
	testwait.Until(t, func() bool {
		reconnect := session.View().Reconnect
		return reconnect != nil && reconnect.WaitingForNetwork
	})
	if !session.Live() {
		t.Fatal("the limit of one attempt ended the session while there was no network")
	}
	failAttempt(t, spy, 2, errNoRoute)
	testwait.Until(t, func() bool { return spy.count() == 4 })
	spy.at(3).finishOpen(nil)
	testwait.Until(t, func() bool { return session.View().State == terminal.StateConnected })
}

func TestAttemptsThatFailForAnotherReasonStillCount(t *testing.T) {
	spy := &readyOpenSpy{}
	registry, _ := newFastRegistry()
	registry.ReconnectLimit = func() int { return 1 }
	session := openReady(t, registry, spy)

	spy.at(0).exit(transportLost)
	failAttempt(t, spy, 1, errors.New("connection refused"))
	testwait.Until(t, func() bool { return !session.Live() })
	if view := session.View(); view.Problem != terminal.ProblemReconnectExhausted {
		t.Errorf("Problem = %q", view.Problem)
	}
}

func TestStoppingTheWaitForTheNetworkLeavesASessionToReconnectByHand(t *testing.T) {
	spy := &readyOpenSpy{}
	registry, _ := newFastRegistry()
	registry.Network = terminal.NewNetworkWatch()
	session := openReady(t, registry, spy)

	registry.Network.SetAvailable(false)
	spy.at(0).exit(transportLost)
	testwait.Until(t, func() bool { return session.View().State == terminal.StateReconnecting })
	if err := registry.StopReconnecting(context.Background(), session.ID()); err != nil {
		t.Fatal(err)
	}
	if view := session.View(); view.State != terminal.StateExited || view.Problem != terminal.ProblemReconnectStopped {
		t.Errorf("view = %+v", view)
	}
}

// 待つ間の試みには印が付く。回線が無いままなら、接続を開く側は理由を書き足さない。
func TestAttemptsWhileWaitingForTheNetworkAreMarked(t *testing.T) {
	marks := make(chan bool, 8)
	spy := &readyOpenSpy{}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", ReconnectError: classifyNoNetwork,
		Open: func(ctx context.Context, size terminal.Size) (terminal.Process, error) {
			// 試みは、試験がそれを知る前に記録され、すぐに spy.at へ進む。
			process, err := spy.open(ctx, size)
			marks <- terminal.IsWaitingForNetwork(ctx)
			return process, err
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	<-marks
	spy.at(0).finishOpen(nil)
	testwait.Until(t, func() bool { return session.View().State == terminal.StateConnected })

	spy.at(0).exit(transportLost)
	if <-marks {
		t.Error("the first attempt was marked: nothing has said there is no network yet")
	}
	failAttempt(t, spy, 1, errNoRoute)
	if !<-marks {
		t.Error("the attempt after one that found no network was not marked")
	}
	spy.at(2).finishOpen(nil)
	testwait.Until(t, func() bool { return session.View().State == terminal.StateConnected })
}

func TestANilNetworkWatchAlwaysHasANetwork(t *testing.T) {
	var watch *terminal.NetworkWatch
	if available, changed := watch.State(); !available || changed != nil {
		t.Errorf("State() = %v, %v", available, changed)
	}
}

// checkedProcess は、輸送を確かめるよう頼まれた回数を数える。
type checkedProcess struct {
	*fakeProcess
	checks atomic.Int32
}

func (p *checkedProcess) CheckTransport(time.Duration) bool {
	p.checks.Add(1)
	return true
}

// Wi-Fi とモバイル回線の切り替えでは、前の回線の TCP が黙って止まる。回線があると言われたら確かめる。
// 回線が無いと言われただけでは確かめない。すぐ戻れば同じ TCP のまま続けられる。
func TestAConnectionIsCheckedWhenTheOperatingSystemSaysANetworkIsThere(t *testing.T) {
	process := &checkedProcess{fakeProcess: newFakeProcess()}
	registry, _ := newFastRegistry()
	registry.Network = terminal.NewNetworkWatch()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "web", Title: "web",
		Open: func(context.Context, terminal.Size) (terminal.Process, error) { return process, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	testwait.Until(t, func() bool { return session.View().State == terminal.StateConnected })

	registry.Network.SetAvailable(false)
	time.Sleep(20 * time.Millisecond)
	if got := process.checks.Load(); got != 0 {
		t.Fatalf("checked %d times with no network", got)
	}
	registry.Network.SetAvailable(true)
	testwait.Until(t, func() bool { return process.checks.Load() == 1 })
	registry.Network.SetAvailable(true)
	testwait.Until(t, func() bool { return process.checks.Load() == 2 })
}
