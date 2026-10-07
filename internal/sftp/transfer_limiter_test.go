package sftp

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

func TestParallelTransfersConsumeOneAggregateSpeedBudget(t *testing.T) {
	limiter := newTransferLimiter()
	const bytesPerSecond = 4096
	const connections = 4
	const payloadBytes = 256
	limiter.setRate(bytesPerSecond)
	started := time.Now()
	var workers sync.WaitGroup
	for range connections {
		workers.Go(func() {
			writer := &limitedTransferWriter{ctx: t.Context(), destination: io.Discard, limiter: limiter}
			if _, err := writer.Write(make([]byte, payloadBytes)); err != nil {
				t.Error(err)
			}
		})
	}
	workers.Wait()
	minimum := time.Duration(connections*payloadBytes)*time.Second/bytesPerSecond - transferSpeedBurstWindow
	if elapsed := time.Since(started); elapsed < minimum {
		t.Fatalf("shared transfer took %s; expected at least %s", elapsed, minimum)
	}
}

func TestCancelledSpeedWaitDoesNotReserveTheNextTransfersCapacity(t *testing.T) {
	limiter := newTransferLimiter()
	limiter.setRate(1)
	ctx, cancel := context.WithCancel(t.Context())
	finished := make(chan error, 1)
	go func() { _, err := limiter.take(ctx, 1); finished <- err }()
	select {
	case err := <-finished:
		t.Fatalf("a rate of one byte/second did not wait: %v", err)
	case <-time.After(transferSpeedBurstWindow):
	}
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled wait = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not interrupt the limiter wait")
	}
	limiter.setRate(0)
	if grant, err := limiter.take(t.Context(), transferSpeedQuantum); err != nil || grant != transferSpeedQuantum {
		t.Fatalf("unlimited grant = %d, %v", grant, err)
	}
}

func TestChangingSpeedToUnlimitedWakesAWaitingTransfer(t *testing.T) {
	limiter := newTransferLimiter()
	limiter.setRate(1)
	finished := make(chan error, 1)
	go func() { _, err := limiter.take(t.Context(), transferSpeedQuantum); finished <- err }()
	limiter.setRate(0)
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("settings change did not wake the limiter")
	}
}

func TestEngineShutdownInterruptsASpeedWaitWithoutARequestCancellation(t *testing.T) {
	limiter := newTransferLimiter()
	limiter.setRate(1)
	finished := make(chan error, 1)
	go func() { _, err := limiter.take(context.Background(), transferSpeedQuantum); finished <- err }()
	limiter.close()
	select {
	case err := <-finished:
		if !errors.Is(err, ErrUnavailable) {
			t.Fatalf("shutdown wait = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown did not wake the speed wait")
	}
	// Live settings and a late refund cannot reopen or double-close the limiter.
	limiter.setRate(0)
	limiter.refund(transferSpeedQuantum)
	if _, err := limiter.take(context.Background(), 1); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("shutdown limiter was reopened: %v", err)
	}
}

func TestReconnectDelayIsBoundedAndRefusalsAreNotTransportFailures(t *testing.T) {
	if reconnectDelay(1) != initialReconnectDelay || reconnectDelay(MaxReconnectAttempts) != maxReconnectDelay {
		t.Fatal("backoff is not bounded")
	}
	for _, err := range []error{ErrConflict, ErrAmbiguousTransfer, context.Canceled, errors.New("ssh: unable to authenticate")} {
		if ConnectionLost(err) {
			t.Fatalf("non-transport failure was recoverable: %v", err)
		}
	}
	if !ConnectionLost(io.EOF) {
		t.Fatal("closed SFTP connection was not recoverable")
	}
}
