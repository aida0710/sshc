// Package releasewatch retains release checks for the lifetime of an engine.
package releasewatch

import (
	"context"
	"errors"
	"sync"
	"time"

	"sshc/internal/releasecheck"
)

// CheckInterval finds new releases without spending the unauthenticated GitHub quota on page loads.
const CheckInterval = 6 * time.Hour

// checkTimeout bounds both GitHub requests even when the HTTP client has no timeout.
const checkTimeout = 30 * time.Second

type Snapshot struct {
	releasecheck.Catalog
	CheckedAt time.Time
	Failure   string
}

type Fetch func(context.Context) (releasecheck.Catalog, error)

type releaseCheck struct {
	done     chan struct{}
	snapshot Snapshot
}

type Watcher struct {
	ctx      context.Context
	cancel   context.CancelFunc
	fetch    Fetch
	mutex    sync.Mutex
	snapshot Snapshot
	checking *releaseCheck
}

func New(parent context.Context, fetch Fetch) *Watcher {
	ctx, cancel := context.WithCancel(parent)
	return &Watcher{ctx: ctx, cancel: cancel, fetch: fetch}
}

// Load checks only when no completed attempt has been retained, including failed attempts.
func (w *Watcher) Load(ctx context.Context) (Snapshot, error) {
	return w.load(ctx, false)
}

// Cached returns the retained result without starting a network request.
func (w *Watcher) Cached() Snapshot {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	return cloneSnapshot(w.snapshot)
}

// Check joins an ongoing check so concurrent requests share its exact result.
func (w *Watcher) Check(ctx context.Context) (Snapshot, error) {
	return w.load(ctx, true)
}

func (w *Watcher) load(ctx context.Context, refresh bool) (Snapshot, error) {
	w.mutex.Lock()
	if !refresh && !w.snapshot.CheckedAt.IsZero() {
		snapshot := cloneSnapshot(w.snapshot)
		w.mutex.Unlock()
		return snapshot, nil
	}
	if err := w.ctx.Err(); err != nil {
		w.mutex.Unlock()
		return Snapshot{}, err
	}
	pending := w.checking
	if pending == nil {
		pending = &releaseCheck{done: make(chan struct{})}
		w.checking = pending
		go w.fetchCatalog(pending)
	}
	w.mutex.Unlock()
	select {
	case <-ctx.Done():
		return Snapshot{}, ctx.Err()
	case <-pending.done:
		return cloneSnapshot(pending.snapshot), nil
	}
}

func (w *Watcher) fetchCatalog(pending *releaseCheck) {
	ctx, cancel := context.WithTimeout(w.ctx, checkTimeout)
	defer cancel()
	catalog, err := w.fetch(ctx)
	w.mutex.Lock()
	defer w.mutex.Unlock()
	snapshot := w.snapshot
	snapshot.CheckedAt = time.Now().UTC()
	snapshot.Failure = ""
	if err == nil || errors.Is(err, releasecheck.ErrNoRelease) {
		snapshot.Catalog = catalog
	} else {
		// A failed refresh must not discard the last successful release and notes.
		snapshot.Failure = err.Error()
	}
	w.snapshot = cloneSnapshot(snapshot)
	pending.snapshot = cloneSnapshot(snapshot)
	w.checking = nil
	close(pending.done)
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	snapshot.Releases = append([]releasecheck.Release(nil), snapshot.Releases...)
	return snapshot
}

// Run checks at startup and periodically until the engine stops. Go's synctest
// clock drives the ticker and timestamps in tests without waiting six real hours.
func (w *Watcher) Run(ctx context.Context) {
	ticker := time.NewTicker(CheckInterval)
	defer ticker.Stop()
	_, _ = w.Load(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			_, _ = w.Check(ctx)
		}
	}
}

// Stop cancels the shared request and waits for it before the engine releases its lock.
func (w *Watcher) Stop() {
	w.cancel()
	w.mutex.Lock()
	pending := w.checking
	w.mutex.Unlock()
	if pending != nil {
		<-pending.done
	}
}
