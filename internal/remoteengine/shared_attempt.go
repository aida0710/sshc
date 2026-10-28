package remoteengine

import (
	"context"
	"sync"
	"time"
)

// sharedAttempt lets callers that arrive while an attempt is running wait for
// its result instead of starting their own. Without it, requests queue behind a
// lock and each repeats a connection that already failed, so a host that does
// not answer keeps the screen waiting once per queued request.
//
// The attempt outlives a caller that gives up: it runs without the caller's
// cancellation, and only that caller stops waiting. It ends at its own limit
// instead, so a host that never answers cannot keep every later caller waiting
// on the same attempt.
type sharedAttempt[T any] struct {
	mutex   sync.Mutex
	running *attemptResult[T]
}

type attemptResult[T any] struct {
	done   chan struct{}
	cancel context.CancelFunc
	value  T
	err    error
}

// do joins the running attempt, or starts one that is cancelled after limit.
// It returns when the attempt ends or when ctx does, whichever comes first.
func (s *sharedAttempt[T]) do(ctx context.Context, limit time.Duration, start func(context.Context) (T, error)) (T, error) {
	s.mutex.Lock()
	attempt := s.running
	if attempt == nil {
		attemptContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), limit)
		attempt = &attemptResult[T]{done: make(chan struct{}), cancel: cancel}
		s.running = attempt
		go s.run(attemptContext, attempt, start)
	}
	s.mutex.Unlock()
	select {
	case <-attempt.done:
		return attempt.value, attempt.err
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
}

func (s *sharedAttempt[T]) run(ctx context.Context, attempt *attemptResult[T], start func(context.Context) (T, error)) {
	attempt.value, attempt.err = start(ctx)
	attempt.cancel()
	s.mutex.Lock()
	s.running = nil
	s.mutex.Unlock()
	close(attempt.done)
}

// abandon cancels the running attempt, if any. Its callers receive the error
// the attempt ends with.
func (s *sharedAttempt[T]) abandon() {
	s.mutex.Lock()
	attempt := s.running
	s.mutex.Unlock()
	if attempt != nil {
		attempt.cancel()
	}
}
