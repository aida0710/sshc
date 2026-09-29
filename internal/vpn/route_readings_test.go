package vpn

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeContainers は、docker の代わりに、コンテナの一覧を答える。答えるまで待たせる
// こともでき、何回読まれたかを数える。
type fakeContainers struct {
	reads      atomic.Int32
	containers map[string]bool
	err        error
	// hold が閉じるまで答えない。nil なら、すぐに答える。
	hold chan struct{}
}

func (fake *fakeContainers) read(ctx context.Context) (map[string]bool, error) {
	fake.reads.Add(1)
	if fake.hold != nil {
		select {
		case <-fake.hold:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return fake.containers, fake.err
}

// steppedClock は、検査が進める時計である。
type steppedClock struct {
	mutex sync.Mutex
	at    time.Time
}

func (clock *steppedClock) now() time.Time {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()
	return clock.at
}

func (clock *steppedClock) advance(duration time.Duration) {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()
	clock.at = clock.at.Add(duration)
}

func managerReading(t *testing.T, fake *fakeContainers) (*Manager, *steppedClock) {
	t.Helper()
	manager := New(t.TempDir(), 1000, nil)
	t.Cleanup(manager.Close)
	clock := &steppedClock{at: time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)}
	manager.now = clock.now
	manager.readContainers = fake.read
	return manager, clock
}

// 同じ時間に来た読み取りは、docker を1回だけ呼び、その結果を分け合う。
func TestReadingsThatArriveTogetherShareOneDockerCall(t *testing.T) {
	fake := &fakeContainers{containers: map[string]bool{"lab": true}, hold: make(chan struct{})}
	manager, _ := managerReading(t, fake)

	var readers sync.WaitGroup
	results := make(chan map[string]Status, 3)
	for range 3 {
		readers.Go(func() {
			statuses, err := manager.Statuses(context.Background())
			if err != nil {
				t.Errorf("Statuses = %v", err)
			}
			results <- statuses
		})
	}
	// 読み始めてから答える。答えより後に来た読み取りも、時計が進んでいないので
	// 同じ結果を使い回す。
	for fake.reads.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	close(fake.hold)
	readers.Wait()
	close(results)

	if reads := fake.reads.Load(); reads != 1 {
		t.Fatalf("docker を %d 回呼んだ, want 1", reads)
	}
	for statuses := range results {
		if !statuses["lab"].Running {
			t.Fatalf("statuses = %+v", statuses)
		}
	}
}

// 読んだ状態は routeReadingLifetime のあいだ使い回し、過ぎたら読み直す。
func TestAReadingIsReusedForItsLifetimeOnly(t *testing.T) {
	fake := &fakeContainers{containers: map[string]bool{}}
	manager, clock := managerReading(t, fake)

	for range 3 {
		if _, err := manager.Statuses(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if reads := fake.reads.Load(); reads != 1 {
		t.Fatalf("使い回せるあいだに docker を %d 回呼んだ", reads)
	}
	clock.advance(routeReadingLifetime)
	if _, err := manager.Statuses(context.Background()); err != nil {
		t.Fatal(err)
	}
	if reads := fake.reads.Load(); reads != 2 {
		t.Fatalf("古くなった状態を読み直さなかった: %d 回", reads)
	}
}

// Docker が使えないことも同じ長さだけ覚える。読み取りのたびに docker info を呼ばない。
func TestAnUnavailableDockerIsRememberedForTheLifetime(t *testing.T) {
	fake := &fakeContainers{err: ErrDockerNotRunning}
	manager, _ := managerReading(t, fake)

	for range 2 {
		if _, err := manager.Statuses(context.Background()); !errors.Is(err, ErrDockerNotRunning) {
			t.Fatalf("Statuses = %v, want ErrDockerNotRunning", err)
		}
	}
	if reads := fake.reads.Load(); reads != 1 {
		t.Fatalf("docker を %d 回呼んだ, want 1", reads)
	}
}

// 確かめる前は待たずに「まだ分からない」と答え、読み始めておく。読み終えたら答える。
func TestKnownStatusesDoNotWaitForDocker(t *testing.T) {
	fake := &fakeContainers{containers: map[string]bool{"lab": true}, hold: make(chan struct{})}
	manager, _ := managerReading(t, fake)

	statuses, known, err := manager.KnownStatuses()
	if known || err != nil || len(statuses) != 0 {
		t.Fatalf("KnownStatuses = %+v, %v, %v, want unknown", statuses, known, err)
	}
	close(fake.hold)
	// 始めておいた読み取りを待つ。docker は1回だけ呼ぶ。
	if _, err := manager.Statuses(context.Background()); err != nil {
		t.Fatal(err)
	}
	statuses, known, err = manager.KnownStatuses()
	if !known || err != nil || !statuses["lab"].Running {
		t.Fatalf("KnownStatuses = %+v, %v, %v", statuses, known, err)
	}
	if reads := fake.reads.Load(); reads != 1 {
		t.Fatalf("docker を %d 回呼んだ, want 1", reads)
	}
}

// sshcエンジンが読んだあとで停止した経路は、docker を読み直さずに停止中として返す。
// 読む前に停止した経路は、docker の状態を使う。
func TestARouteTheEngineStoppedAfterTheReadingIsShownStopped(t *testing.T) {
	fake := &fakeContainers{containers: map[string]bool{"lab": true, "earlier": true}}
	manager, _ := managerReading(t, fake)
	manager.noteChange(manager.state("earlier"))

	if statuses, err := manager.Statuses(context.Background()); err != nil || !statuses["lab"].Running ||
		!statuses["earlier"].Running {
		t.Fatalf("Statuses = %+v, %v", statuses, err)
	}
	// 経路を停止した（state.running は偽のまま）。
	manager.noteChange(manager.state("lab"))

	statuses, err := manager.Statuses(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, present := statuses["lab"]; present {
		t.Fatalf("停止した経路を、覚えた状態のまま動いていると返した: %+v", statuses["lab"])
	}
	if !statuses["earlier"].Running {
		t.Fatalf("読む前に変わった経路は、docker の状態を使う: %+v", statuses["earlier"])
	}
	if reads := fake.reads.Load(); reads != 1 {
		t.Fatalf("docker を %d 回呼んだ, want 1", reads)
	}
}

// 捨てる前に読み始めたものは、読み終えても覚えない（止めたコンテナが残って見えない）。
func TestAReadingStartedBeforeItWasForgottenIsNotKept(t *testing.T) {
	fake := &fakeContainers{containers: map[string]bool{"orphan": true}, hold: make(chan struct{})}
	manager, _ := managerReading(t, fake)

	if _, known, _ := manager.KnownStatuses(); known {
		t.Fatal("読む前から分かっていた")
	}
	manager.forgetRoutes()
	fake.containers = map[string]bool{}
	close(fake.hold)

	statuses, err := manager.Statuses(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, present := statuses["orphan"]; present {
		t.Fatalf("捨てる前に読み始めた状態を覚えた: %+v", statuses)
	}
}

// docker を探して使えなかった直後は、経路の状態を読むときに探し直さない。経路の
// 起動のように利用者が求めた操作は、その場で探し直す。
func TestAFailedDockerLookupIsReusedByReadsButNotByActions(t *testing.T) {
	manager := New(t.TempDir(), 1000, nil)
	t.Cleanup(manager.Close)
	clock := &steppedClock{at: time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)}
	manager.now = clock.now
	// 環境は一度読むと覚えるので、検査では読み込み済みの印（loaded）を戻し、探す
	// たびに環境を読ませる。環境を読んだ回数が、docker を探した回数になる。
	lookups := 0
	manager.environment = func(context.Context) ([]string, error) {
		lookups++
		// docker の無い PATH。探すたびに ErrDockerMissing になる。
		return []string{"PATH=" + t.TempDir()}, nil
	}
	if _, err := manager.command(context.Background()); !errors.Is(err, ErrDockerMissing) {
		t.Fatalf("command = %v", err)
	}
	manager.loaded = false
	if _, err := manager.commandForReading(context.Background()); !errors.Is(err, ErrDockerMissing) {
		t.Fatalf("commandForReading = %v", err)
	}
	if lookups != 1 {
		t.Fatalf("直前に失敗したのに、読むために探し直した（%d 回）", lookups)
	}
	if _, err := manager.command(context.Background()); !errors.Is(err, ErrDockerMissing) {
		t.Fatalf("command = %v", err)
	}
	if lookups != 2 {
		t.Fatalf("利用者が求めた操作で探し直さなかった（%d 回）", lookups)
	}
	clock.advance(routeReadingLifetime)
	manager.loaded = false
	if _, err := manager.commandForReading(context.Background()); !errors.Is(err, ErrDockerMissing) {
		t.Fatalf("commandForReading = %v", err)
	}
	if lookups != 3 {
		t.Fatalf("古くなった失敗で、探し直さなかった（%d 回）", lookups)
	}
}
