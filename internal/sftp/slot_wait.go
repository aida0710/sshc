package sftp

import (
	"context"
	"time"
)

// staleSweepGrace is added to the moment a silent running job reaches
// staleRunningTransferAfter. The sweep counts a job as stale only once that
// limit has strictly passed.
const staleSweepGrace = 10 * time.Millisecond

// A remote job refused a slot waits for the next change that can let it
// start instead of asking again on a timer. Releasing a running job and
// changing the settings close the current channel, and every waiting worker
// asks again.

// signalSlotLocked wakes the remote workers waiting for a slot.
func (m *TransferManager) signalSlotLocked() {
	if m.slotReleased != nil {
		close(m.slotReleased)
		m.slotReleased = nil
	}
}

// slotWait returns the channel closed the next time a slot may open. A
// running job that stops reporting frees its slot only when a sweep notices
// it, so sweepAfter says when the oldest such job would be swept (0 when
// there is none). Take it before asking for the slot, so a release in
// between is not missed.
func (m *TransferManager) slotWait() (slotReleased <-chan struct{}, sweepAfter time.Duration) {
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	if m.slotReleased == nil {
		m.slotReleased = make(chan struct{})
	}
	return m.slotReleased, m.untilStaleSweepLocked()
}

func (m *TransferManager) untilStaleSweepLocked() time.Duration {
	now := m.now().UTC()
	soonest := time.Duration(0)
	for _, record := range m.jobs {
		if record.job.Status != TransferRunning || m.dataPlane[record.job.ID] > 0 {
			continue
		}
		until := max(record.job.UpdatedAt.Add(staleRunningTransferAfter).Sub(now), 0) + staleSweepGrace
		if soonest == 0 || until < soonest {
			soonest = until
		}
	}
	return soonest
}

// waitForSlot reports false when ctx ends first.
func waitForSlot(ctx context.Context, slotReleased <-chan struct{}, sweepAfter time.Duration) bool {
	var sweep <-chan time.Time
	if sweepAfter > 0 {
		timer := time.NewTimer(sweepAfter)
		defer timer.Stop()
		sweep = timer.C
	}
	select {
	case <-ctx.Done():
		return false
	case <-slotReleased:
	case <-sweep:
	}
	return true
}
