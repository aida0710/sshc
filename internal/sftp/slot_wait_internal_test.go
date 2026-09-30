package sftp

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAWaiterWakesWhenTheOldestSilentRunningJobWouldBeSwept(t *testing.T) {
	started := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	now := started.Add(time.Minute)
	manager := NewTransferManager(&Service{}, t.TempDir())
	t.Cleanup(func() { _ = manager.Close() })
	manager.ConfigureJobs(4, func() time.Time { return now })
	manager.jobsMutex.Lock()
	manager.jobs = map[string]*transferJobRecord{
		"transfer_silent0001": {job: TransferJob{ID: "transfer_silent0001", Status: TransferRunning, UpdatedAt: started}},
		"transfer_recent0001": {job: TransferJob{ID: "transfer_recent0001", Status: TransferRunning, UpdatedAt: now}},
		"transfer_active0001": {job: TransferJob{ID: "transfer_active0001", Status: TransferRunning, UpdatedAt: started.Add(-time.Hour)}},
		"transfer_queued0001": {job: TransferJob{ID: "transfer_queued0001", Status: TransferQueued, UpdatedAt: started.Add(-time.Hour)}},
	}
	// A job whose data plane is alive is never swept.
	manager.dataPlane = map[string]int{"transfer_active0001": 1}
	manager.jobsMutex.Unlock()

	_, sweepAfter := manager.slotWait()
	if want := staleRunningTransferAfter - time.Minute + staleSweepGrace; sweepAfter != want {
		t.Fatalf("sweep after %v, want %v", sweepAfter, want)
	}
}

func TestAStaleSweepThatCannotBeSavedLeavesTheQueueAsItWas(t *testing.T) {
	started := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	now := started.Add(staleRunningTransferAfter + time.Second)
	manager := NewTransferManager(&Service{}, t.TempDir())
	t.Cleanup(func() { _ = manager.Close() })
	manager.ConfigureJobs(4, func() time.Time { return now })
	notADirectory := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(notADirectory, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	const id = "transfer_silent0001"
	manager.jobsMutex.Lock()
	manager.jobs = map[string]*transferJobRecord{
		id: {job: TransferJob{ID: id, Status: TransferRunning, UpdatedAt: started}},
	}
	manager.jobOrder = []string{id}
	manager.activeJobs = 1
	manager.queuePath = filepath.Join(notADirectory, "transfers.json")
	manager.jobsMutex.Unlock()

	if _, err := manager.ListJobs(); err == nil {
		t.Fatal("ListJobs saved a queue into a path under a regular file")
	}
	manager.jobsMutex.Lock()
	defer manager.jobsMutex.Unlock()
	if job := manager.jobs[id].job; job.Status != TransferRunning || job.Problem != "" || !job.UpdatedAt.Equal(started) {
		t.Fatalf("job after the failed save = %+v, want it still running as before", job)
	}
	if manager.activeJobs != 1 {
		t.Fatalf("active jobs = %d, want the slot still held", manager.activeJobs)
	}
}
