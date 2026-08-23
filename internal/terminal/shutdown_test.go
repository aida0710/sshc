package terminal_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"sshc/internal/terminal"
)

type stubbornProcess struct {
	blockHangup  chan struct{}
	ignoreHangup bool
	releaseDone  chan struct{}
	closeOnForce bool

	forces atomic.Int32
	closed sync.Once
}

func newStubbornProcess(closeOnForce bool) *stubbornProcess {
	return &stubbornProcess{
		blockHangup:  make(chan struct{}),
		releaseDone:  make(chan struct{}),
		closeOnForce: closeOnForce,
	}
}

func (p *stubbornProcess) Read(b []byte) (int, error) {
	<-p.releaseDone
	return 0, errors.New("terminal closed")
}

func (p *stubbornProcess) Write(b []byte) (int, error) { return len(b), nil }
func (p *stubbornProcess) Resize(terminal.Size) error  { return nil }

func (p *stubbornProcess) Hangup() error {
	if p.ignoreHangup {
		return nil
	}
	<-p.blockHangup
	return nil
}

func (p *stubbornProcess) Wait() terminal.ExitInfo {
	<-p.releaseDone
	return terminal.ExitInfo{Code: -1}
}

func (p *stubbornProcess) Close() error { return nil }

func (p *stubbornProcess) ForceClose() error {
	p.forces.Add(1)
	if p.closeOnForce {
		p.release()
	}
	return nil
}

func (p *stubbornProcess) release() { p.closed.Do(func() { close(p.releaseDone) }) }

func (p *stubbornProcess) forceCount() int { return int(p.forces.Load()) }

func openStubborn(t *testing.T, registry *terminal.Registry, process terminal.Process) {
	t.Helper()
	if _, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "stubborn",
		Open: func(context.Context, terminal.Size) (terminal.Process, error) { return process, nil },
	}); err != nil {
		t.Fatalf("Open = %v", err)
	}
}

func TestForceCloseReachesEveryProcessWhileOneHangupIsBlocked(t *testing.T) {
	registry := &terminal.Registry{Limits: func() terminal.Limits { return terminal.DefaultLimits() }}

	blocking := newStubbornProcess(true)
	ignoring := newStubbornProcess(true)
	ignoring.ignoreHangup = true
	broken := newStubbornProcess(false)
	broken.ignoreHangup = true

	for _, process := range []*stubbornProcess{blocking, ignoring, broken} {
		openStubborn(t, registry, process)
	}

	registry.BeginShutdown()

	waited := make(chan error, 1)
	go func() { waited <- registry.Wait() }()

	registry.ForceClose()

	for name, process := range map[string]*stubbornProcess{
		"blocking Hangup": blocking, "ignored Hangup": ignoring, "never closing done": broken,
	} {
		deadline := time.Now().Add(2 * time.Second)
		for process.forceCount() == 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if got := process.forceCount(); got != 1 {
			t.Fatalf("%s force hook ran %d times, want exactly 1", name, got)
		}
	}

	select {
	case err := <-waited:
		t.Fatalf("Wait returned while a process had not finished: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	close(blocking.blockHangup)
	broken.release()

	select {
	case err := <-waited:
		if err != nil {
			t.Fatalf("Wait = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Wait did not return after every process finished")
	}
}

func TestBeginShutdownCancelsAPendingOpenWithoutWaitingForIt(t *testing.T) {
	registry := &terminal.Registry{Limits: func() terminal.Limits { return terminal.DefaultLimits() }}

	entered := make(chan struct{})
	opened := make(chan error, 1)
	go func() {
		_, err := registry.Open(context.Background(), terminal.Spec{
			Kind: terminal.KindSSH, Alias: "slow",
			Open: func(ctx context.Context, _ terminal.Size) (terminal.Process, error) {
				close(entered)
				<-ctx.Done()
				return nil, ctx.Err()
			},
		})
		opened <- err
	}()
	<-entered

	done := make(chan struct{})
	go func() { registry.BeginShutdown(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("BeginShutdown waited for the pending creation")
	}

	select {
	case err := <-opened:
		if err == nil {
			t.Fatal("the cancelled creation reported success")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the pending creation was never cancelled")
	}

	if err := registry.Wait(); err != nil {
		t.Fatalf("Wait = %v", err)
	}
	if sessions := registry.Sessions(); len(sessions) != 0 {
		t.Fatalf("sessions = %#v, want none", sessions)
	}
}

func TestALateProcessIsForcedAndNeverPublished(t *testing.T) {
	registry := &terminal.Registry{Limits: func() terminal.Limits { return terminal.DefaultLimits() }}

	entered := make(chan struct{})
	proceed := make(chan struct{})
	late := newStubbornProcess(true)
	opened := make(chan error, 1)
	go func() {
		_, err := registry.Open(context.Background(), terminal.Spec{
			Kind: terminal.KindSSH, Alias: "late",
			Open: func(context.Context, terminal.Size) (terminal.Process, error) {
				close(entered)
				<-proceed
				return late, nil
			},
		})
		opened <- err
	}()
	<-entered

	registry.BeginShutdown()
	close(proceed)

	if err := <-opened; !errors.Is(err, terminal.ErrShuttingDown) {
		t.Fatalf("late Open = %v, want ErrShuttingDown", err)
	}
	if sessions := registry.Sessions(); len(sessions) != 0 {
		t.Fatalf("sessions = %#v, want none", sessions)
	}
	if late.forceCount() == 0 {
		t.Fatal("the late process was published or dropped without being forced")
	}
	if err := registry.Wait(); err != nil {
		t.Fatalf("Wait = %v", err)
	}
}

func TestOpenAfterBeginShutdownIsRefused(t *testing.T) {
	registry := &terminal.Registry{Limits: func() terminal.Limits { return terminal.DefaultLimits() }}
	registry.BeginShutdown()
	_, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "after",
		Open: func(context.Context, terminal.Size) (terminal.Process, error) {
			t.Fatal("a session was created after shutdown began")
			return nil, nil
		},
	})
	if !errors.Is(err, terminal.ErrShuttingDown) {
		t.Fatalf("Open after BeginShutdown = %v, want ErrShuttingDown", err)
	}
}

func TestWaitAdmitsForceCloseRegisteredAfterItWasEntered(t *testing.T) {
	registry := &terminal.Registry{Limits: func() terminal.Limits { return terminal.DefaultLimits() }}
	process := newStubbornProcess(true)
	process.ignoreHangup = true
	openStubborn(t, registry, process)

	registry.BeginShutdown()

	var group sync.WaitGroup
	group.Add(2)
	waited := make(chan error, 1)
	go func() {
		defer group.Done()
		waited <- registry.Wait()
	}()
	go func() {
		defer group.Done()
		process.release()
		registry.ForceClose()
	}()
	group.Wait()

	select {
	case err := <-waited:
		if err != nil {
			t.Fatalf("Wait = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Wait did not return")
	}
	if err := registry.Wait(); err != nil {
		t.Fatalf("second Wait = %v", err)
	}
}

func TestShutdownCallsAreIdempotentAndConcurrencySafe(t *testing.T) {
	registry := &terminal.Registry{Limits: func() terminal.Limits { return terminal.DefaultLimits() }}
	process := newStubbornProcess(true)
	process.ignoreHangup = true
	openStubborn(t, registry, process)

	var group sync.WaitGroup
	results := make([]error, 8)
	for index := range results {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			registry.BeginShutdown()
			registry.ForceClose()
			results[index] = registry.Wait()
		}(index)
	}
	group.Wait()
	for index, err := range results {
		if err != nil {
			t.Fatalf("concurrent Wait %d = %v", index, err)
		}
	}
	if got := process.forceCount(); got != 1 {
		t.Fatalf("force hook ran %d times, want exactly 1", got)
	}
}
