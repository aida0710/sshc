package sftp

import (
	"context"
	"errors"
	"testing"
	"time"
)

// stubRemote answers only what the pool itself calls.
type stubRemote struct {
	Remote
	alive  bool
	closed bool
}

func (r *stubRemote) Getwd(context.Context) (string, error) {
	if !r.alive {
		return "", errors.New("transport closed")
	}
	return "/", nil
}

func (r *stubRemote) Close() error {
	r.closed = true
	return nil
}

type reportingRemote struct {
	*stubRemote
}

func (r *reportingRemote) Alive() bool { return r.alive }

type poolHarness struct {
	pool    *RemotePool
	dialled []*stubRemote
	clock   time.Time
	timers  []func()
}

func newPoolHarness(t *testing.T, alive func(*stubRemote) Remote) *poolHarness {
	t.Helper()
	harness := &poolHarness{clock: time.Date(2026, 9, 19, 3, 0, 0, 0, time.UTC)}
	harness.pool = NewRemotePool(func(context.Context, string) (RemoteTarget, error) {
		return RemoteTarget{Identity: "unchanged", Open: func(context.Context) (Remote, error) {
			remote := &stubRemote{alive: true}
			harness.dialled = append(harness.dialled, remote)
			return alive(remote), nil
		}}, nil
	})
	harness.pool.now = func() time.Time { return harness.clock }
	harness.pool.after = func(_ time.Duration, expire func()) *time.Timer {
		harness.timers = append(harness.timers, expire)
		return nil
	}
	return harness
}

func plain(remote *stubRemote) Remote     { return remote }
func reporting(remote *stubRemote) Remote { return &reportingRemote{stubRemote: remote} }

func (h *poolHarness) fireTimers() {
	timers := h.timers
	h.timers = nil
	for _, expire := range timers {
		expire()
	}
}

func TestPoolHandsTheReturnedConnectionToTheNextOperation(t *testing.T) {
	harness := newPoolHarness(t, plain)
	first, err := harness.pool.Open(context.Background(), "edge")
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := harness.pool.Open(context.Background(), "edge")
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if len(harness.dialled) != 1 || harness.dialled[0].closed {
		t.Fatalf("dialled %d connections, first closed = %v", len(harness.dialled), harness.dialled[0].closed)
	}
	if _, err := harness.pool.Open(context.Background(), "other"); err != nil {
		t.Fatal(err)
	}
	if len(harness.dialled) != 2 {
		t.Fatalf("another host reused a connection: dialled %d", len(harness.dialled))
	}
}

func TestPoolClosesAConnectionThatSatIdleForTheTimeout(t *testing.T) {
	harness := newPoolHarness(t, plain)
	remote, err := harness.pool.Open(context.Background(), "edge")
	if err != nil {
		t.Fatal(err)
	}
	_ = remote.Close()
	harness.clock = harness.clock.Add(RemoteIdleTimeout)
	harness.fireTimers()
	if !harness.dialled[0].closed {
		t.Fatal("idle connection was kept past the timeout")
	}
	if _, err := harness.pool.Open(context.Background(), "edge"); err != nil {
		t.Fatal(err)
	}
	if len(harness.dialled) != 2 {
		t.Fatalf("dialled %d, want a fresh connection after expiry", len(harness.dialled))
	}
}

func TestPoolDoesNotHandOutAConnectionTheHostDropped(t *testing.T) {
	for name, wrap := range map[string]func(*stubRemote) Remote{"asked": plain, "reported": reporting} {
		t.Run(name, func(t *testing.T) {
			harness := newPoolHarness(t, wrap)
			remote, err := harness.pool.Open(context.Background(), "edge")
			if err != nil {
				t.Fatal(err)
			}
			_ = remote.Close()
			harness.dialled[0].alive = false
			replacement, err := harness.pool.Open(context.Background(), "edge")
			if err != nil {
				t.Fatal(err)
			}
			defer replacement.Close()
			if len(harness.dialled) != 2 || !harness.dialled[0].closed {
				t.Fatalf("dialled %d, dead one closed = %v", len(harness.dialled), harness.dialled[0].closed)
			}
		})
	}
}

func TestACancelledOperationDiscardsItsConnection(t *testing.T) {
	harness := newPoolHarness(t, plain)
	ctx, cancel := context.WithCancel(context.Background())
	remote, err := harness.pool.Open(ctx, "edge")
	if err != nil {
		t.Fatal(err)
	}
	bound := bindRemoteContext(ctx, remote)
	cancel()
	// Cancellation closes through the context; the later Close is a no-op.
	_ = bound.Close()
	if !harness.dialled[0].closed {
		t.Fatal("the cancelled operation's connection was not closed")
	}
	if _, err := harness.pool.Open(context.Background(), "edge"); err != nil {
		t.Fatal(err)
	}
	if len(harness.dialled) != 2 {
		t.Fatalf("a discarded connection was handed out again: dialled %d", len(harness.dialled))
	}
}

func TestPoolKeepsAFewIdleConnectionsPerHostAndClosesTheRest(t *testing.T) {
	harness := newPoolHarness(t, plain)
	var opened []Remote
	for index := 0; index < maxIdleRemotesPerHost+2; index++ {
		remote, err := harness.pool.Open(context.Background(), "edge")
		if err != nil {
			t.Fatal(err)
		}
		opened = append(opened, remote)
	}
	for _, remote := range opened {
		_ = remote.Close()
	}
	closed := 0
	for _, remote := range harness.dialled {
		if remote.closed {
			closed++
		}
	}
	if closed != 2 {
		t.Fatalf("closed %d of %d, want the surplus 2", closed, len(harness.dialled))
	}
	if err := harness.pool.Close(); err != nil {
		t.Fatal(err)
	}
	for _, remote := range harness.dialled {
		if !remote.closed {
			t.Fatal("pool Close left a connection open")
		}
	}
	late, err := harness.pool.Open(context.Background(), "edge")
	if err != nil {
		t.Fatal(err)
	}
	_ = late.Close()
	if !harness.dialled[len(harness.dialled)-1].closed {
		t.Fatal("a connection released after Close was pooled")
	}
}
