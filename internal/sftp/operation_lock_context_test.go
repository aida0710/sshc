package sftp

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"
)

// Failed cancellation must fail the test without leaving it blocked forever.
const operationLockTestDeadline = 5 * time.Second

func TestCancelledOperationLockReturnsBeforeTheHeldPathIsReleased(t *testing.T) {
	service := Service{}
	manager := NewTransferManager(&service, t.TempDir())
	defer manager.Close()
	unlock, err := manager.LockOperation("edge", "/a")
	if err != nil {
		t.Fatal(err)
	}
	release := sync.OnceFunc(unlock)
	defer release()
	deadline, stopDeadline := context.WithTimeout(t.Context(), operationLockTestDeadline)
	defer stopDeadline()
	ctx, cancel := context.WithCancel(deadline)
	defer cancel()
	outcome := make(chan error, 1)
	go func() {
		unlock, err := manager.LockOperationContext(ctx, "edge", "/a")
		if unlock != nil {
			unlock()
		}
		outcome <- err
	}()
	// Observe that the second operation has entered the same lock's wait,
	// so cancellation is tested after acquisition starts rather than before it.
	for {
		manager.mutex.Lock()
		waiting := manager.locks["edge\x00/a"].refs > 1
		manager.mutex.Unlock()
		if waiting {
			break
		}
		select {
		case err := <-outcome:
			t.Fatalf("wait ended before cancellation: %v", err)
		case <-deadline.Done():
			t.Fatal("operation never entered the lock wait")
		default:
			runtime.Gosched()
		}
	}
	cancel()
	select {
	case err := <-outcome:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled wait = %v", err)
		}
	case <-deadline.Done():
		t.Fatal("cancelled operation waited for the held path")
	}
	release()
	// The cancelled acquisition must release any lock it obtains later.
	secondUnlock, err := manager.LockOperationContext(deadline, "edge", "/a")
	if err != nil {
		t.Fatal(err)
	}
	secondUnlock()
}
