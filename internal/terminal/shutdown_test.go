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

// stubbornProcess は、止まらない相手の三つの形を演じる。
//
// **これが停止処理の本当の相手である。** 応答しないリモートに向いた ssh は
// Hangup の中で返らず、SIGHUP を無視する子は終わらず、壊れたアダプタは
// 終わったことを言わない。
type stubbornProcess struct {
	// blockHangup が閉じられるまで Hangup は返らない。
	blockHangup chan struct{}
	// ignoreHangup が真なら、Hangup は即座に返るが何も起こさない。
	ignoreHangup bool
	// releaseDone が閉じられるまで、読み取りは終わらない——つまり done も
	// 閉じない。強制停止だけがこれを解ける。
	releaseDone chan struct{}
	// closeOnForce が真なら、強制停止が読み取りを終わらせる。偽なら、
	// 強制されても終わらない壊れたアダプタである。
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

// ForceClose は、レジストリが任意の追加契約として見つける強制停止である。
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

// **返らない一本が、他のすべてを止めてはならない。**
//
// 締切に達したら、強制停止はどのセッションに対しても始まる。Hangup の中で
// 止まっているものが居ても、無視するものが居ても、終わったことを言わない
// ものが居ても、である。そして合流は、全部が本当に終わるまで返らない。
func TestForceCloseReachesEveryProcessWhileOneHangupIsBlocked(t *testing.T) {
	registry := &terminal.Registry{Limits: func() terminal.Limits { return terminal.DefaultLimits() }}

	blocking := newStubbornProcess(true)
	ignoring := newStubbornProcess(true)
	ignoring.ignoreHangup = true
	// 強制されても done を閉じない、壊れたアダプタ。
	broken := newStubbornProcess(false)
	broken.ignoreHangup = true

	for _, process := range []*stubbornProcess{blocking, ignoring, broken} {
		openStubborn(t, registry, process)
	}

	registry.BeginShutdown()

	waited := make(chan error, 1)
	go func() { waited <- registry.Wait() }()

	// 締切に相当する合図。coordinator が呼ぶのと同じものである。
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

	// **壊れたアダプタが居るあいだ、合流は返らない。** ここで返してしまうと、
	// engine lock を手放したあとに状態を変える処理がまだ走っていることになる。
	select {
	case err := <-waited:
		t.Fatalf("Wait returned while a process had not finished: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	// 止まっていた Hangup を解き、壊れたアダプタを終わらせる。
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

// 確保の途中で止まっている Open が居ても、停止は錠を取れる。
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

	// **待たずに返らなければならない。** ここで確保の完了を待つと、応答しない
	// 相手への接続が停止そのものを止める。
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

// 停止のあとに返ってきた Process は、畳まれる。**一覧に載せてはならない。**
// 載せれば、停止処理が数え終えたあとに生きたセッションが現れる。
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

// **合流は、入った後に足された仕事も待たなければならない。**
//
// 最後のセッションが締切とちょうど同時に終わると、数え上げは一瞬ゼロになる。
// そこで返してしまう作りだと、まだ走っている強制停止と、engine lock を
// 手放したあとの処理が重なる。
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
		// セッションの終了と強制停止を同じ瞬間にぶつける。
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
	// 送り出した強制停止が本当に終わっていること。
	if err := registry.Wait(); err != nil {
		t.Fatalf("second Wait = %v", err)
	}
}

// 停止の呼び出しは、何度でも、同時に来てもよい。
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
