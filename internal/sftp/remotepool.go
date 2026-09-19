package sftp

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"
)

// RemotePool keeps a host's SFTP connection open for a short while after the
// operation that used it finishes, so that the next operation on the same host
// skips the SSH handshake, and with it every interactive step such as a
// one-time code. A pooled connection serves one operation at a time. Closing
// the connection an operation was given returns it to the pool; an operation
// whose context is cancelled discards it instead, because closing the
// transport is the only way to interrupt an SFTP request that is blocked.
//
// A pooled connection outlives the operation that authenticated it, in the
// same way an open terminal does: locking the vault stops new connections,
// not the ones already made, and an idle one closes after RemoteIdleTimeout.

const (
	// RemoteIdleTimeout is how long an unused connection stays open: long
	// enough to cover a person moving between directories or the CLI queueing
	// the next file, short enough that a forgotten pane does not hold a
	// session on the host for hours.
	RemoteIdleTimeout = 60 * time.Second
	// maxIdleRemotesPerHost bounds what a burst of parallel range downloads
	// leaves behind for one host.
	maxIdleRemotesPerHost = 4
)

// A remote that can say whether its transport is still up lets the pool skip
// a round trip before reuse. Without it the pool asks the server instead.
type livenessReporter interface {
	Alive() bool
}

// A remote that a cancelled operation must close for real rather than return.
type discardableRemote interface {
	Discard() error
}

type idleRemote struct {
	remote Remote
	since  time.Time
}

type RemotePool struct {
	open  OpenRemote
	now   func() time.Time
	after func(time.Duration, func()) *time.Timer

	mutex  sync.Mutex
	idle   map[string][]idleRemote
	closed bool
}

func NewRemotePool(open OpenRemote) *RemotePool {
	return &RemotePool{open: open, now: time.Now, after: time.AfterFunc, idle: map[string][]idleRemote{}}
}

// Open hands out an idle connection to the host when one is still alive, and
// dials otherwise.
func (p *RemotePool) Open(ctx context.Context, alias string) (Remote, error) {
	for {
		remote := p.takeIdle(alias)
		if remote == nil {
			break
		}
		if p.alive(ctx, remote) {
			return p.wrap(alias, remote), nil
		}
		_ = remote.Close()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	remote, err := p.open(ctx, alias)
	if err != nil {
		return nil, err
	}
	return p.wrap(alias, remote), nil
}

// Close shuts every idle connection and makes later releases close theirs.
func (p *RemotePool) Close() error {
	p.mutex.Lock()
	p.closed = true
	var remotes []Remote
	for alias, entries := range p.idle {
		for _, entry := range entries {
			remotes = append(remotes, entry.remote)
		}
		delete(p.idle, alias)
	}
	p.mutex.Unlock()
	var joined []error
	for _, remote := range remotes {
		if err := remote.Close(); err != nil {
			joined = append(joined, err)
		}
	}
	return errors.Join(joined...)
}

func (p *RemotePool) takeIdle(alias string) Remote {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	entries := p.idle[alias]
	if len(entries) == 0 {
		return nil
	}
	// The most recently released connection is the least likely to have been
	// dropped by the host in the meantime.
	last := entries[len(entries)-1]
	p.idle[alias] = entries[:len(entries)-1]
	return last.remote
}

func (p *RemotePool) alive(ctx context.Context, remote Remote) bool {
	if reporter, ok := remote.(livenessReporter); ok {
		return reporter.Alive()
	}
	_, err := remote.Getwd(ctx)
	return err == nil
}

// release puts a connection back for the next operation, or closes it when
// the pool is closed or the host already has enough idle connections.
func (p *RemotePool) release(alias string, remote Remote) error {
	p.mutex.Lock()
	if p.closed {
		p.mutex.Unlock()
		return remote.Close()
	}
	var evicted Remote
	entries := p.idle[alias]
	if len(entries) >= maxIdleRemotesPerHost {
		evicted = entries[0].remote
		entries = entries[1:]
	}
	since := p.now()
	p.idle[alias] = append(entries, idleRemote{remote: remote, since: since})
	p.mutex.Unlock()
	p.after(RemoteIdleTimeout, func() { p.expire(alias, remote) })
	if evicted != nil {
		return evicted.Close()
	}
	return nil
}

// expire closes a connection that has sat idle since it was released. One
// that was taken and released again in the meantime has a newer timer.
func (p *RemotePool) expire(alias string, remote Remote) {
	p.mutex.Lock()
	entries := p.idle[alias]
	index := -1
	for candidate, entry := range entries {
		if entry.remote == remote && !p.now().Before(entry.since.Add(RemoteIdleTimeout)) {
			index = candidate
		}
	}
	if index >= 0 {
		p.idle[alias] = append(entries[:index:index], entries[index+1:]...)
	}
	p.mutex.Unlock()
	if index >= 0 {
		_ = remote.Close()
	}
}

func (p *RemotePool) wrap(alias string, remote Remote) Remote {
	pooled := &pooledRemote{Remote: remote, pool: p, alias: alias}
	if _, ok := remote.(RangeRemote); ok {
		return &pooledRangeRemote{pooledRemote: pooled}
	}
	return pooled
}

// pooledRemote is what an operation holds: Close returns the connection to
// the pool, Discard closes it for real.
type pooledRemote struct {
	Remote
	pool  *RemotePool
	alias string
	once  sync.Once
	err   error
}

func (r *pooledRemote) Close() error {
	r.once.Do(func() { r.err = r.pool.release(r.alias, r.Remote) })
	return r.err
}

func (r *pooledRemote) Discard() error {
	r.once.Do(func() { r.err = r.Remote.Close() })
	return r.err
}

type pooledRangeRemote struct{ *pooledRemote }

func (r *pooledRangeRemote) OpenRange(candidate string, offset int64) (io.ReadCloser, error) {
	ranged, ok := r.Remote.(RangeRemote)
	if !ok {
		return nil, ErrInvalidTransfer
	}
	return ranged.OpenRange(candidate, offset)
}
