package sftp

import (
	"context"
	"io"
	"sync"
	"time"
)

const (
	// Keep the limit representable in the API and bound timer arithmetic.
	MaxTransferSpeedBytesPerSecond = int64(1 << 30)
	// Small grants bound read-ahead and keep cancellation responsive at low rates.
	transferSpeedQuantum     = 32 << 10
	transferSpeedBurstWindow = 50 * time.Millisecond
)

// transferLimiter shares one payload budget across every engine transfer,
// including independent jobs and range connections. Waiting never reserves
// future capacity, so cancelling a waiter cannot delay the next transfer.
type transferLimiter struct {
	mutex   sync.Mutex
	rate    int64
	tokens  float64
	updated time.Time
	changed chan struct{}
	closed  bool
}

func newTransferLimiter() *transferLimiter {
	return &transferLimiter{updated: time.Now(), changed: make(chan struct{})}
}

func (limiter *transferLimiter) setRate(rate int64) {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	if limiter.closed || limiter.rate == rate {
		return
	}
	limiter.rate, limiter.tokens, limiter.updated = rate, 0, time.Now()
	close(limiter.changed)
	limiter.changed = make(chan struct{})
}

func (limiter *transferLimiter) close() {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	if limiter.closed {
		return
	}
	limiter.closed = true
	close(limiter.changed)
}

func speedGrant(rate int64) int {
	if rate == 0 {
		return transferSpeedQuantum
	}
	return int(max(1, min(int64(transferSpeedQuantum), int64(float64(rate)*transferSpeedBurstWindow.Seconds()))))
}

// take returns a grant rather than accepting an arbitrarily large write.
// Every wait observes cancellation and live settings changes.
func (limiter *transferLimiter) take(ctx context.Context, wanted int) (int, error) {
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		limiter.mutex.Lock()
		if limiter.closed {
			limiter.mutex.Unlock()
			return 0, ErrUnavailable
		}
		rate := limiter.rate
		if rate == 0 {
			limiter.mutex.Unlock()
			return wanted, nil
		}
		now := time.Now()
		grant := min(wanted, speedGrant(rate))
		limiter.tokens = min(float64(speedGrant(rate)), limiter.tokens+now.Sub(limiter.updated).Seconds()*float64(rate))
		limiter.updated = now
		if limiter.tokens >= float64(grant) {
			limiter.tokens -= float64(grant)
			limiter.mutex.Unlock()
			return grant, nil
		}
		delay := time.Duration((float64(grant) - limiter.tokens) / float64(rate) * float64(time.Second))
		changed := limiter.changed
		limiter.mutex.Unlock()
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return 0, ctx.Err()
		case <-changed:
			timer.Stop()
		case <-timer.C:
		}
	}
}

func (limiter *transferLimiter) refund(unused int) {
	if unused <= 0 {
		return
	}
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	if limiter.closed || limiter.rate == 0 {
		return
	}
	// Short reads, EOF and failed writes consume only the payload actually
	// transferred. Wake other waiters rather than making them wait for a refill.
	limiter.tokens = min(float64(speedGrant(limiter.rate)), limiter.tokens+float64(unused))
	close(limiter.changed)
	limiter.changed = make(chan struct{})
}

type limitedTransferWriter struct {
	ctx         context.Context
	destination io.Writer
	limiter     *transferLimiter
}

func (writer *limitedTransferWriter) Write(contents []byte) (int, error) {
	written := 0
	for len(contents) > 0 {
		grant, err := writer.limiter.take(writer.ctx, len(contents))
		if err != nil {
			return written, err
		}
		count, err := writer.destination.Write(contents[:grant])
		writer.limiter.refund(grant - count)
		written += count
		if err != nil {
			return written, err
		}
		if count != grant {
			return written, io.ErrShortWrite
		}
		contents = contents[count:]
	}
	return written, nil
}

func (s Service) transferWriter(ctx context.Context, destination io.Writer) io.Writer {
	if s.transferLimiter == nil {
		return destination
	}
	return &limitedTransferWriter{ctx: ctx, destination: destination, limiter: s.transferLimiter}
}

func (download *PreparedDownload) transferWriter(ctx context.Context, destination io.Writer) io.Writer {
	if download.limiter == nil {
		return destination
	}
	return &limitedTransferWriter{ctx: ctx, destination: destination, limiter: download.limiter}
}

type limitedTransferReader struct {
	ctx     context.Context
	source  io.Reader
	limiter *transferLimiter
}

func (reader *limitedTransferReader) Read(contents []byte) (int, error) {
	if reader.limiter == nil {
		return reader.source.Read(contents)
	}
	if len(contents) == 0 {
		return 0, nil
	}
	grant, err := reader.limiter.take(reader.ctx, len(contents))
	if err != nil {
		return 0, err
	}
	read, err := reader.source.Read(contents[:grant])
	reader.limiter.refund(grant - read)
	return read, err
}

func validTransferSpeed(value int64) bool {
	return value >= 0 && value <= MaxTransferSpeedBytesPerSecond
}

type transferLimiterContextKey struct{}

func (m *TransferManager) withTransferLimiter(ctx context.Context) context.Context {
	return context.WithValue(ctx, transferLimiterContextKey{}, m.limiter)
}

func transferVerificationReader(ctx context.Context, source io.Reader) io.Reader {
	limiter, _ := ctx.Value(transferLimiterContextKey{}).(*transferLimiter)
	if limiter == nil {
		return source
	}
	return &limitedTransferReader{ctx: ctx, source: source, limiter: limiter}
}
