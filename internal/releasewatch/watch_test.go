package releasewatch

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"sshc/internal/releasecheck"
)

func TestRetainsChecksAndRefreshesEverySixHoursUntilStopped(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var calls atomic.Int32
		watcher := New(ctx, func(context.Context) (releasecheck.Catalog, error) {
			calls.Add(1)
			return releasecheck.Catalog{Latest: releasecheck.Release{Version: "v1.1.0"}}, nil
		})
		defer watcher.Stop()
		done := make(chan struct{})
		go func() { defer close(done); watcher.Run(ctx) }()
		synctest.Wait()
		first, err := watcher.Load(ctx)
		if err != nil || first.CheckedAt.IsZero() || first.Latest.Version != "v1.1.0" || calls.Load() != 1 {
			t.Fatalf("startup = %+v, %v, calls=%d", first, err, calls.Load())
		}
		time.Sleep(CheckInterval - time.Second)
		synctest.Wait()
		if calls.Load() != 1 {
			t.Fatal("checked before six hours")
		}
		time.Sleep(time.Second)
		synctest.Wait()
		second, _ := watcher.Load(ctx)
		if calls.Load() != 2 || second.CheckedAt.Sub(first.CheckedAt) != CheckInterval {
			t.Fatalf("periodic = %+v, calls=%d", second, calls.Load())
		}
		watcher.Stop()
		<-done
		time.Sleep(CheckInterval)
		if calls.Load() != 2 {
			t.Fatal("checked after shutdown")
		}
	})
}

func TestConcurrentChecksShareOneResultAndCallerCancellationDoesNotCancelOthers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		var calls atomic.Int32
		watcher := New(context.Background(), func(context.Context) (releasecheck.Catalog, error) {
			calls.Add(1)
			<-release
			return releasecheck.Catalog{Latest: releasecheck.Release{Version: "v1.2.0"}}, nil
		})
		defer watcher.Stop()
		ctx, cancel := context.WithCancel(context.Background())
		cancelled := make(chan error, 1)
		go func() { _, err := watcher.Check(ctx); cancelled <- err }()
		const readers = 8
		snapshots := make(chan Snapshot, readers)
		for range readers {
			go func() { snapshot, _ := watcher.Check(context.Background()); snapshots <- snapshot }()
		}
		synctest.Wait()
		cancel()
		if !errors.Is(<-cancelled, context.Canceled) {
			t.Fatal("caller did not cancel")
		}
		close(release)
		for range readers {
			if snapshot := <-snapshots; snapshot.Latest.Version != "v1.2.0" {
				t.Fatalf("snapshot=%+v", snapshot)
			}
		}
		if calls.Load() != 1 {
			t.Fatalf("calls=%d", calls.Load())
		}
	})
}

func TestFailedRefreshRetainsReleaseNotesAndFailureWithoutRetryingReads(t *testing.T) {
	calls := 0
	watcher := New(context.Background(), func(context.Context) (releasecheck.Catalog, error) {
		calls++
		if calls > 1 {
			return releasecheck.Catalog{}, errors.New("upstream unavailable")
		}
		return releasecheck.Catalog{Latest: releasecheck.Release{Version: "v1.2.0"}, Releases: []releasecheck.Release{{Version: "v1.1.0", Body: "notes"}}}, nil
	})
	defer watcher.Stop()
	first, _ := watcher.Load(context.Background())
	first.Releases[0].Body = "caller mutation"
	failed, _ := watcher.Check(context.Background())
	retained, _ := watcher.Load(context.Background())
	if calls != 2 || retained.Failure != "upstream unavailable" || failed.Latest.Version != "v1.2.0" || retained.Releases[0].Body != "notes" || retained.CheckedAt.Before(first.CheckedAt) {
		t.Fatalf("retained=%+v calls=%d", retained, calls)
	}
}

func TestFirstFailureIsRetainedAndStopCancelsAnOngoingCheck(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		watcher := New(context.Background(), func(ctx context.Context) (releasecheck.Catalog, error) {
			<-ctx.Done()
			return releasecheck.Catalog{}, ctx.Err()
		})
		done := make(chan Snapshot, 1)
		go func() { snapshot, _ := watcher.Load(context.Background()); done <- snapshot }()
		synctest.Wait()
		watcher.Stop()
		snapshot := <-done
		if snapshot.Failure != context.Canceled.Error() || snapshot.CheckedAt.IsZero() {
			t.Fatalf("snapshot=%+v", snapshot)
		}
	})
}

func TestInitialFailureIsNotRetriedByPageReads(t *testing.T) {
	calls := 0
	watcher := New(context.Background(), func(context.Context) (releasecheck.Catalog, error) {
		calls++
		return releasecheck.Catalog{}, errors.New("offline")
	})
	defer watcher.Stop()
	for range 3 {
		snapshot, err := watcher.Load(context.Background())
		if err != nil || snapshot.Failure != "offline" {
			t.Fatalf("snapshot=%+v err=%v", snapshot, err)
		}
	}
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestACheckThatLearnedLessKeepsWhatAnEarlierCheckRead(t *testing.T) {
	published := time.Date(2026, 11, 3, 3, 0, 0, 0, time.UTC)
	read := releasecheck.Catalog{
		Latest:   releasecheck.Release{Version: "v1.1.0", PublishedAt: published},
		Releases: []releasecheck.Release{{Version: "v1.1.0", Body: "notes"}},
	}
	answers := []releasecheck.Catalog{read, {Latest: releasecheck.Release{Version: "v1.1.0", PageURL: "https://example.invalid/v1.1.0"}}}
	watcher := New(t.Context(), func(context.Context) (releasecheck.Catalog, error) {
		answer := answers[0]
		answers = answers[1:]
		return answer, nil
	})
	defer watcher.Stop()
	if _, err := watcher.Check(t.Context()); err != nil {
		t.Fatal(err)
	}
	second, err := watcher.Check(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if second.Failure != "" || !second.Latest.PublishedAt.Equal(published) || len(second.Releases) != 1 {
		t.Errorf("second check = %+v", second)
	}
}

func TestANewerReleaseIsToldEvenWhenItsNotesCouldNotBeRead(t *testing.T) {
	answers := []releasecheck.Catalog{
		{Latest: releasecheck.Release{Version: "v1.1.0", PublishedAt: time.Date(2026, 11, 3, 3, 0, 0, 0, time.UTC)}, Releases: []releasecheck.Release{{Version: "v1.1.0"}}},
		{Latest: releasecheck.Release{Version: "v1.2.0"}},
	}
	watcher := New(t.Context(), func(context.Context) (releasecheck.Catalog, error) {
		answer := answers[0]
		answers = answers[1:]
		return answer, nil
	})
	defer watcher.Stop()
	if _, err := watcher.Check(t.Context()); err != nil {
		t.Fatal(err)
	}
	second, err := watcher.Check(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if second.Latest.Version != "v1.2.0" || !second.Latest.PublishedAt.IsZero() {
		t.Errorf("second check = %+v", second)
	}
}
