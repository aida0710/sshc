package terminal_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"sshc/internal/terminal"
)

type readyProcess struct {
	*fakeProcess
	ready  chan error
	prompt atomic.Bool
}

func newReadyProcess() *readyProcess {
	return &readyProcess{fakeProcess: newFakeProcess(), ready: make(chan error, 1)}
}

func (p *readyProcess) Ready() <-chan error  { return p.ready }
func (p *readyProcess) AwaitingPrompt() bool { return p.prompt.Load() }

func (p *readyProcess) finishOpen(err error) {
	p.ready <- err
	close(p.ready)
}

type readyOpenSpy struct {
	mutex     sync.Mutex
	processes []*readyProcess
}

func (s *readyOpenSpy) open(_ context.Context, _ terminal.Size) (terminal.Process, error) {
	process := newReadyProcess()
	s.mutex.Lock()
	s.processes = append(s.processes, process)
	s.mutex.Unlock()
	return process, nil
}

func (s *readyOpenSpy) at(index int) *readyProcess {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if index >= len(s.processes) {
		return nil
	}
	return s.processes[index]
}

func (s *readyOpenSpy) count() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return len(s.processes)
}

type openSpy struct {
	mutex     sync.Mutex
	processes []*fakeProcess
	calls     int
	failUpTo  int
}

func (s *openSpy) open(_ context.Context, _ terminal.Size) (terminal.Process, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.calls++
	if s.calls > 1 && s.calls <= s.failUpTo {
		return nil, context.DeadlineExceeded
	}
	process := newFakeProcess()
	s.processes = append(s.processes, process)
	return process, nil
}

func (s *openSpy) at(index int) *fakeProcess {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if index >= len(s.processes) {
		return nil
	}
	return s.processes[index]
}

func (s *openSpy) count() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.calls
}

func TestALostTransportIsDialledAgain(t *testing.T) {
	spy := &openSpy{}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}

	spy.at(0).exit(terminal.ExitInfo{Code: terminal.TransportLost})

	waitFor(t, func() bool { return spy.count() >= 2 }) // 繋ぎ直しに行かなかった
	if !session.Live() {
		t.Error("繋ぎ直しているあいだに、終了済みにされた")
	}

	waitFor(t, func() bool {
		return strings.Contains(string(session.Snapshot()), "繋ぎ直しました")
	})
	if !strings.Contains(string(session.Snapshot()), "新しいシェル") {
		t.Error("新しいシェルであることを言っていない: 前の続きだと思わせてはならない")
	}
}

func TestAShellThatExitedIsLeftAlone(t *testing.T) {
	spy := &openSpy{}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}

	spy.at(0).exit(terminal.ExitInfo{Code: 0})

	waitFor(t, func() bool { return !session.Live() }) // 終わらなかった
	if spy.count() != 1 {
		t.Errorf("繋ぎ直しに行った: 呼ばれた回数 %d", spy.count())
	}
}

func TestGivingUpIsSaidOutLoud(t *testing.T) {
	spy := &openSpy{failUpTo: 1 + terminal.MaxReconnects}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}
	spy.at(0).exit(terminal.ExitInfo{Code: terminal.TransportLost})

	waitFor(t, func() bool { return !session.Live() }) // 諦めなかった
	if !strings.Contains(string(session.Snapshot()), "試行上限に達しました") {
		t.Error("諦めたことを言っていない")
	}
	view := session.View()
	if view.State != terminal.StateExited || view.Problem != "reconnect_exhausted" {
		t.Errorf("state/problem = %q/%q, want exited/reconnect_exhausted", view.State, view.Problem)
	}
}

func TestAsynchronousHandshakeFailuresConsumeTheReconnectBudget(t *testing.T) {
	spy := &readyOpenSpy{}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}

	if state := session.View().State; state != terminal.StateConnecting {
		t.Fatalf("initial state = %q, want connecting", state)
	}
	connected := make(chan struct{}, 1)
	session.WhenConnected(func() { connected <- struct{}{} })
	select {
	case <-connected:
		t.Fatal("connected callback ran before the asynchronous handshake")
	default:
	}
	spy.at(0).finishOpen(nil)
	waitFor(t, func() bool { return session.View().State == terminal.StateConnected })
	select {
	case <-connected:
	case <-time.After(time.Second):
		t.Fatal("connected callback did not run after Ready succeeded")
	}
	spy.at(0).exit(terminal.ExitInfo{Code: terminal.TransportLost})

	for attempt := 1; attempt <= terminal.MaxReconnects; attempt++ {
		waitFor(t, func() bool { return spy.count() > attempt })
		candidate := spy.at(attempt)
		candidate.finishOpen(context.DeadlineExceeded)
		candidate.exit(terminal.ExitInfo{Code: 255})
	}

	waitFor(t, func() bool { return !session.Live() })
	if calls := spy.count(); calls != 1+terminal.MaxReconnects {
		t.Fatalf("open calls = %d, want initial + %d reconnects", calls, terminal.MaxReconnects)
	}
	view := session.View()
	if view.State != terminal.StateExited || view.Problem != "reconnect_exhausted" {
		t.Fatalf("state/problem = %q/%q, want exited/reconnect_exhausted", view.State, view.Problem)
	}
}

func TestAsynchronousHandshakeErrorKeepsItsTypedClassification(t *testing.T) {
	classified := errors.New("classified handshake failure")
	spy := &readyOpenSpy{}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
		ReconnectError: func(err error) (bool, string) {
			if errors.Is(err, classified) {
				return false, "host_key_changed"
			}
			return true, "reconnect_failed"
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	spy.at(0).finishOpen(nil)
	waitFor(t, func() bool { return session.View().State == terminal.StateConnected })
	spy.at(0).exit(terminal.ExitInfo{Code: terminal.TransportLost})
	waitFor(t, func() bool { return spy.count() == 2 })
	spy.at(1).finishOpen(fmt.Errorf("wrapped: %w", classified))
	spy.at(1).exit(terminal.ExitInfo{Code: 255})

	waitFor(t, func() bool { return !session.Live() })
	if calls := spy.count(); calls != 2 {
		t.Fatalf("open calls = %d, want classification to stop after one reconnect", calls)
	}
	if problem := session.View().Problem; problem != "host_key_changed" {
		t.Fatalf("problem = %q, want host_key_changed", problem)
	}
}

func TestReconnectStateIsVisibleWhileWaiting(t *testing.T) {
	spy := &openSpy{}
	registry, _ := newRegistry(terminal.DefaultLimits())
	registry.ReconnectDelay = func(int) time.Duration { return time.Hour }
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}
	spy.at(0).exit(terminal.ExitInfo{Code: terminal.TransportLost})

	waitFor(t, func() bool { return session.View().State == terminal.StateReconnecting })
	view := session.View()
	if view.Reconnect == nil || view.Reconnect.Attempt != 1 || view.Reconnect.Limit != terminal.MaxReconnects {
		t.Fatalf("reconnect = %#v", view.Reconnect)
	}
	if view.Reconnect.RetryAt.IsZero() {
		t.Error("次回試行時刻が無い")
	}
	if err := registry.Close(session.ID()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return !session.Live() })
}

func TestReconnectStopsWhenTheFailureNeedsUserAction(t *testing.T) {
	spy := &openSpy{failUpTo: 1 + terminal.MaxReconnects}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
		ReconnectError: func(error) (bool, string) { return false, "host_key_changed" },
	})
	if err != nil {
		t.Fatal(err)
	}
	spy.at(0).exit(terminal.ExitInfo{Code: terminal.TransportLost})

	waitFor(t, func() bool { return !session.Live() })
	if spy.count() != 2 {
		t.Fatalf("open calls = %d, want initial + one reconnect", spy.count())
	}
	view := session.View()
	if view.Problem != "host_key_changed" || view.State != terminal.StateExited {
		t.Fatalf("state/problem = %q/%q", view.State, view.Problem)
	}
}

func TestALocalShellIsNeverDialledAgain(t *testing.T) {
	registry, starter := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindShell, Title: "zsh", Command: terminal.Command{Path: "/bin/sh"},
	})
	if err != nil {
		t.Fatal(err)
	}
	before := len(starter.processes)

	starter.processes[len(starter.processes)-1].exit(terminal.ExitInfo{Code: terminal.TransportLost})

	waitFor(t, func() bool { return !session.Live() }) // 終わらなかった
	if len(starter.processes) != before {
		t.Error("ローカルのシェルを繋ぎ直しに行った")
	}
}

func newFastRegistry() (*terminal.Registry, *fakeStarter) {
	registry, starter := newRegistry(terminal.DefaultLimits())
	registry.ReconnectDelay = func(int) time.Duration { return 0 }
	return registry, starter
}

func TestKeystrokesDuringAReconnectAreDropped(t *testing.T) {
	spy := &openSpy{}
	registry, _ := newRegistry(terminal.DefaultLimits())
	registry.ReconnectDelay = func(int) time.Duration { return 150 * time.Millisecond }
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}
	first := spy.at(0)
	first.exit(terminal.ExitInfo{Code: terminal.TransportLost})

	waitFor(t, func() bool {
		return strings.Contains(string(session.Snapshot()), "繋ぎ直します")
	})
	if _, err := session.Write([]byte("rm -rf /tmp/half")); err != nil {
		t.Fatalf("繋ぎ直しのあいだの打鍵が失敗した: %v", err)
	}

	waitFor(t, func() bool { return spy.count() >= 2 })
	waitFor(t, func() bool {
		return strings.Contains(string(session.Snapshot()), "繋ぎ直しました")
	})

	if got := spy.at(1).keystrokes(); got != "" {
		t.Errorf("溜めた打鍵が新しいシェルへ届いた: %q", got)
	}
}

func TestKeystrokesDuringReconnectHandshakeAreDroppedUntilReady(t *testing.T) {
	spy := &readyOpenSpy{}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}
	first := spy.at(0)
	first.finishOpen(nil)
	waitFor(t, func() bool { return session.View().State == terminal.StateConnected })
	first.exit(terminal.ExitInfo{Code: terminal.TransportLost})

	waitFor(t, func() bool { return spy.count() >= 2 })
	second := spy.at(1)
	waitFor(t, func() bool { return session.View().State == terminal.StateReconnecting })
	if _, err := session.Write([]byte("dangerous-command\r")); err != nil {
		t.Fatalf("Write during handshake = %v", err)
	}
	if got := second.keystrokes(); got != "" {
		t.Fatalf("Ready前の入力が新しいshellへ渡った: %q", got)
	}

	second.finishOpen(nil)
	second.exit(terminal.ExitInfo{Code: 0})
}

func TestAuthenticationPromptStillAcceptsInputBeforeReady(t *testing.T) {
	spy := &readyOpenSpy{}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}
	process := spy.at(0)
	process.prompt.Store(true)
	if _, err := session.Write([]byte("answer\r")); err != nil {
		t.Fatalf("Write to prompt = %v", err)
	}
	if got := process.keystrokes(); got != "answer\r" {
		t.Fatalf("prompt answer = %q", got)
	}
	process.finishOpen(context.Canceled)
	process.exit(terminal.ExitInfo{Code: terminal.TransportLost})
}

type closingSpy struct {
	mutex     sync.Mutex
	processes []*fakeProcess
}

type reclaimedProcess struct {
	*fakeProcess
	closed chan struct{}
	once   sync.Once
}

func newReclaimedProcess() *reclaimedProcess {
	return &reclaimedProcess{fakeProcess: newFakeProcess(), closed: make(chan struct{})}
}

func (p *reclaimedProcess) Close() error {
	p.once.Do(func() {
		p.fakeProcess.exit(terminal.ExitInfo{Code: -1})
		close(p.closed)
	})
	return nil
}

func (p *reclaimedProcess) ForceClose() error { return p.Close() }

type blockingReconnectSpy struct {
	initial     *fakeProcess
	entered     chan struct{}
	replacement chan *reclaimedProcess
	calls       atomic.Int32
}

func newBlockingReconnectSpy() *blockingReconnectSpy {
	return &blockingReconnectSpy{
		initial: newFakeProcess(), entered: make(chan struct{}),
		replacement: make(chan *reclaimedProcess, 1),
	}
}

func (s *blockingReconnectSpy) open(ctx context.Context, _ terminal.Size) (terminal.Process, error) {
	if s.calls.Add(1) == 1 {
		return s.initial, nil
	}
	close(s.entered)
	<-ctx.Done()
	process := newReclaimedProcess()
	s.replacement <- process
	return process, nil
}

func TestStoppingDuringReconnectOpenCancelsAndReclaimsTheReturnedProcess(t *testing.T) {
	for _, test := range []struct {
		name string
		stop func(*terminal.Registry, *terminal.Session) error
	}{
		{name: "close", stop: func(registry *terminal.Registry, session *terminal.Session) error {
			return registry.Close(session.ID())
		}},
		{name: "shutdown", stop: func(registry *terminal.Registry, _ *terminal.Session) error {
			registry.BeginShutdown()
			return registry.Wait()
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			spy := newBlockingReconnectSpy()
			registry, _ := newFastRegistry()
			session, err := registry.Open(context.Background(), terminal.Spec{
				Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
			})
			if err != nil {
				t.Fatal(err)
			}
			spy.initial.exit(terminal.ExitInfo{Code: terminal.TransportLost})
			select {
			case <-spy.entered:
			case <-time.After(time.Second):
				t.Fatal("reconnect Open was not entered")
			}

			stopped := make(chan error, 1)
			go func() { stopped <- test.stop(registry, session) }()
			var replacement *reclaimedProcess
			select {
			case replacement = <-spy.replacement:
			case <-time.After(time.Second):
				t.Fatal("reconnect context was not cancelled")
			}
			select {
			case <-replacement.closed:
			case <-time.After(time.Second):
				t.Fatal("Process returned after stop was not reclaimed")
			}
			select {
			case err := <-stopped:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("stop did not join the reconnect lifecycle")
			}
			waitFor(t, func() bool { return !session.Live() })
			if state := session.View().State; state == terminal.StateConnected {
				t.Fatal("a Process returned after stop was published as connected")
			}
		})
	}
}

func (s *closingSpy) open(_ context.Context, _ terminal.Size) (terminal.Process, error) {
	process := newFakeProcess()
	process.onHangup = func(p *fakeProcess) { p.exit(terminal.ExitInfo{Code: terminal.TransportLost}) }
	s.mutex.Lock()
	s.processes = append(s.processes, process)
	s.mutex.Unlock()
	return process, nil
}

func (s *closingSpy) count() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return len(s.processes)
}

func TestClosingAConsoleDoesNotPromiseToDialAgain(t *testing.T) {
	spy := &closingSpy{}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := registry.Close(session.ID()); err != nil {
		t.Fatal(err)
	}

	waitFor(t, func() bool { return !session.Live() })
	if strings.Contains(string(session.Snapshot()), "繋ぎ直します") {
		t.Errorf("closed session announced a reconnect that will not occur:\n%s", session.Snapshot())
	}
	if spy.count() != 1 {
		t.Errorf("開き直しに行った回数 = %d, want 1（閉じたのだから行かない）", spy.count())
	}
}

func TestAConsoleThePersonClosedLeavesTheList(t *testing.T) {
	spy := &closingSpy{}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Close(session.ID()); err != nil {
		t.Fatal(err)
	}

	waitFor(t, func() bool { return len(registry.Sessions()) == 0 })
}

func TestAConsoleThatDroppedStaysToBeRead(t *testing.T) {
	spy := &openSpy{}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}

	for attempt := 0; attempt <= terminal.MaxReconnects; attempt++ {
		waitFor(t, func() bool { return spy.count() >= attempt+1 })
		spy.at(attempt).exit(terminal.ExitInfo{Code: terminal.TransportLost})
	}

	waitFor(t, func() bool { return !session.Live() })
	if len(registry.Sessions()) != 1 {
		t.Errorf("sessions = %d, want the dropped console kept so its reason can be read", len(registry.Sessions()))
	}
}

func TestChoosingNoReconnectEndsTheSessionAtOnce(t *testing.T) {
	spy := &openSpy{}
	registry, _ := newFastRegistry()
	registry.Reconnects = func() int { return 0 }
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}

	spy.at(0).exit(terminal.ExitInfo{Code: terminal.TransportLost})

	waitFor(t, func() bool { return !session.Live() })
	if spy.count() != 1 {
		t.Errorf("開き直しを %d 回試みた。0 を選んだのに繋ぎ直している", spy.count())
	}
	if strings.Contains(string(session.Snapshot()), "試行上限に達しました") {
		t.Errorf("繋ぎ直さない設定なのに、諦めたと書いた:\n%s", session.Snapshot())
	}
}

func TestLoweringTheReconnectCountStopsASessionAlreadyTrying(t *testing.T) {
	spy := &openSpy{failUpTo: 99}
	registry, _ := newRegistry(terminal.DefaultLimits())
	registry.ReconnectDelay = func(int) time.Duration { return 60 * time.Millisecond }

	var allowed atomic.Int64
	allowed.Store(int64(terminal.MaxReconnects))
	registry.Reconnects = func() int { return int(allowed.Load()) }

	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}

	spy.at(0).exit(terminal.ExitInfo{Code: terminal.TransportLost})
	waitFor(t, func() bool { return spy.count() >= 2 })
	if !session.Live() {
		t.Fatal("まだ粘っているはずが、もう終わっていた")
	}

	allowed.Store(0)
	waitFor(t, func() bool { return !session.Live() })
}

func TestTheReconnectWindowIsCountedFromTheGaps(t *testing.T) {
	if window := terminal.ReconnectWindow(0); window != 0 {
		t.Errorf("0 回 = %v, want 0", window)
	}
	for attempts, want := range map[int]time.Duration{
		1: 2 * time.Second,
		2: 4 * time.Second,
		3: 10 * time.Second,
		5: 40 * time.Second,
	} {
		if window := terminal.ReconnectWindow(attempts); window != want {
			t.Errorf("%d 回 = %v, want %v", attempts, window, want)
		}
	}
	if terminal.NormaliseReconnects(-1) != terminal.MaxReconnects ||
		terminal.NormaliseReconnects(99) != terminal.MaxReconnects {
		t.Error("範囲の外が既定へ戻っていない")
	}
	if terminal.NormaliseReconnects(0) != 0 {
		t.Error("0 が既定へ戻された。切る道が無くなる")
	}
}
