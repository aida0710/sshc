package remoteengine

import (
	"context"
	"sync"
)

// sharedAttempt lets callers that arrive while an attempt is running wait for
// its result instead of starting their own. Without it, requests queue behind a
// lock and each repeats a connection that already failed, so a host that does
// not answer keeps the screen waiting once per queued request.
//
// The attempt outlives a caller that gives up: it runs without the caller's
// cancellation, and only that caller stops waiting.
type sharedAttempt[T any] struct {
	mutex   sync.Mutex
	running *attemptResult[T]
}

type attemptResult[T any] struct {
	done  chan struct{}
	value T
	err   error
}

func (s *sharedAttempt[T]) do(ctx context.Context, start func(context.Context) (T, error)) (T, error) {
	s.mutex.Lock()
	attempt := s.running
	if attempt == nil {
		attempt = &attemptResult[T]{done: make(chan struct{})}
		s.running = attempt
		go s.run(context.WithoutCancel(ctx), attempt, start)
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
	s.mutex.Lock()
	s.running = nil
	s.mutex.Unlock()
	close(attempt.done)
}
