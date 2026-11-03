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
	resolve ResolveRemote
	now     func() time.Time
	after   func(time.Duration, func()) *time.Timer

	mutex sync.Mutex
	idle  map[string][]idleRemote
	// inUse is the connections operations have taken, by key, which Borrow can lend as well.
	inUse map[string][]Remote
	// borrowers counts the reads a connection is lent to; none of them keeps it from an operation, but the idle timer
	// waits for them.
	borrowers map[Remote]int
	closed    bool
}

func NewRemotePool(resolve ResolveRemote) *RemotePool {
	return &RemotePool{
		resolve: resolve, now: time.Now, after: time.AfterFunc,
		idle: map[string][]idleRemote{}, inUse: map[string][]Remote{}, borrowers: map[Remote]int{},
	}
}

// Open resolves the current configuration before reusing a connection. The
// opener captures that same target, so a concurrent edit cannot change its peer.
func (p *RemotePool) Open(ctx context.Context, alias string) (Remote, error) {
	target, err := p.resolve(ctx, alias)
	if err != nil {
		return nil, err
	}
	key := alias + "\x00" + target.Identity
	for {
		remote := p.takeIdle(key)
		if remote == nil {
			break
		}
		if p.alive(ctx, remote) {
			return p.wrap(key, remote), nil
		}
		p.markFinished(key, remote)
		_ = remote.Close()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	remote, err := target.Open(ctx)
	if err != nil {
		return nil, err
	}
	p.mutex.Lock()
	p.inUse[key] = append(p.inUse[key], remote)
	p.mutex.Unlock()
	return p.wrap(key, remote), nil
}

// Borrow lends a short read the connection a host has open, idle or in use, and never dials. Unlike Open it does not
// take the connection away: the next operation finds it in the pool as usual and runs beside the read on the same SFTP
// session, which serves requests side by side. Nothing the read does can close the connection for the others, except a
// Discard after it has stopped responding, and if an operation closes it, the read simply fails.
func (p *RemotePool) Borrow(ctx context.Context, alias string) (Remote, error) {
	target, err := p.resolve(ctx, alias)
	if err != nil {
		return nil, err
	}
	key := alias + "\x00" + target.Identity
	remote := p.lend(key)
	if remote == nil {
		return nil, ErrNoOpenConnection
	}
	if !p.alive(ctx, remote) {
		p.giveBack(key, remote)
		return nil, ErrNoOpenConnection
	}
	return &borrowedRemote{Remote: remote, pool: p, key: key}, nil
}

// lend prefers the connection released last, as takeIdle does, and otherwise one an operation is using.
func (p *RemotePool) lend(key string) Remote {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if p.closed {
		return nil
	}
	var remote Remote
	if entries := p.idle[key]; len(entries) > 0 {
		remote = entries[len(entries)-1].remote
	} else if used := p.inUse[key]; len(used) > 0 {
		remote = used[len(used)-1]
	}
	if remote != nil {
		p.borrowers[remote]++
	}
	return remote
}

// giveBack ends a loan. A connection still idle has been used just now, so its idle time starts over.
func (p *RemotePool) giveBack(key string, remote Remote) {
	p.mutex.Lock()
	if p.borrowers[remote] > 1 {
		p.borrowers[remote]--
		p.mutex.Unlock()
		return
	}
	delete(p.borrowers, remote)
	restarted := false
	for index := range p.idle[key] {
		if p.idle[key][index].remote == remote {
			p.idle[key][index].since = p.now()
			restarted = true
		}
	}
	p.mutex.Unlock()
	if restarted {
		p.after(RemoteIdleTimeout, func() { p.expire(key, remote) })
	}
}

// borrowedRemote is what a borrower holds: Close ends the loan and leaves the connection open.
type borrowedRemote struct {
	Remote
	pool *RemotePool
	key  string
	once sync.Once
}

func (r *borrowedRemote) Close() error {
	r.once.Do(func() { r.pool.giveBack(r.key, r.Remote) })
	return nil
}

// Discard closes the connection for everyone, for a borrower whose read has stopped responding. The pool drops it the
// next time it looks, as it does a connection the host has closed.
func (r *borrowedRemote) Discard() error {
	return r.Remote.Close()
}

// Close shuts every idle connection and makes later releases close theirs.
func (p *RemotePool) Close() error {
	p.mutex.Lock()
	p.closed = true
	var remotes []Remote
	for key, entries := range p.idle {
		for _, entry := range entries {
			remotes = append(remotes, entry.remote)
		}
		delete(p.idle, key)
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

func (p *RemotePool) takeIdle(key string) Remote {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	entries := p.idle[key]
	if len(entries) == 0 {
		return nil
	}
	// The most recently released connection is the least likely to have been
	// dropped by the host in the meantime.
	last := entries[len(entries)-1]
	p.idle[key] = entries[:len(entries)-1]
	p.inUse[key] = append(p.inUse[key], last.remote)
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
func (p *RemotePool) release(key string, remote Remote) error {
	p.mutex.Lock()
	if p.closed {
		p.forgetInUse(key, remote)
		p.mutex.Unlock()
		return remote.Close()
	}
	p.forgetInUse(key, remote)
	var evicted Remote
	entries := p.idle[key]
	if len(entries) >= maxIdleRemotesPerHost {
		evicted = entries[0].remote
		entries = entries[1:]
	}
	since := p.now()
	p.idle[key] = append(entries, idleRemote{remote: remote, since: since})
	p.mutex.Unlock()
	p.after(RemoteIdleTimeout, func() { p.expire(key, remote) })
	if evicted != nil {
		return evicted.Close()
	}
	return nil
}

func (p *RemotePool) markFinished(key string, remote Remote) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.forgetInUse(key, remote)
}

func (p *RemotePool) forgetInUse(key string, remote Remote) {
	used := p.inUse[key]
	for index, candidate := range used {
		if candidate == remote {
			p.inUse[key] = append(used[:index:index], used[index+1:]...)
			return
		}
	}
}

// expire closes a connection that has sat idle since it was released. One that was taken and released again in the
// meantime has a newer timer, and so does one that is lent out, when it comes back.
func (p *RemotePool) expire(key string, remote Remote) {
	p.mutex.Lock()
	entries := p.idle[key]
	index := -1
	for candidate, entry := range entries {
		if entry.remote == remote && p.borrowers[remote] == 0 && !p.now().Before(entry.since.Add(RemoteIdleTimeout)) {
			index = candidate
		}
	}
	if index >= 0 {
		p.idle[key] = append(entries[:index:index], entries[index+1:]...)
	}
	p.mutex.Unlock()
	if index >= 0 {
		_ = remote.Close()
	}
}

func (p *RemotePool) wrap(key string, remote Remote) Remote {
	pooled := &pooledRemote{Remote: remote, pool: p, key: key}
	if _, ok := remote.(RangeRemote); ok {
		return &pooledRangeRemote{pooledRemote: pooled}
	}
	return pooled
}

// pooledRemote is what an operation holds: Close returns the connection to
// the pool, Discard closes it for real.
type pooledRemote struct {
	Remote
	pool *RemotePool
	key  string
	once sync.Once
	err  error
}

func (r *pooledRemote) Close() error {
	r.once.Do(func() { r.err = r.pool.release(r.key, r.Remote) })
	return r.err
}

func (r *pooledRemote) Discard() error {
	r.once.Do(func() {
		r.pool.markFinished(r.key, r.Remote)
		r.err = r.Remote.Close()
	})
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
