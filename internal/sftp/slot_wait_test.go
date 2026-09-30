package sftp_test

import (
	"sync/atomic"
	"testing"
	"time"

	"sshc/internal/sftp"
)

// holdTheOnlySlot starts a download the test controls, so a remote job
// scheduled afterwards has to wait.
func holdTheOnlySlot(t *testing.T, manager *sftp.TransferManager) string {
	t.Helper()
	const slotHolder = "transfer_slot_holder"
	if _, err := manager.CreateJob(sftp.CreateTransferJob{
		ID: slotHolder, BatchID: "batch_slot_holder", Alias: "edge",
		Direction: sftp.TransferDownload, Kind: sftp.TransferFile, Name: "held.bin", RemotePath: "/held.bin", TotalBytes: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob(slotHolder, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	return slotHolder
}

func TestAWaitingRemoteJobDoesNotAskForASlotUntilOneOpens(t *testing.T) {
	source := remoteWith(map[string]node{"/source.bin": file("source.bin", "payload", 0o644)})
	manager := newCopyJobManager(t, source, remoteWith(nil))
	// Every request to the queue reads the clock, so its reads count them.
	var queueRequests atomic.Int64
	manager.ConfigureJobs(1, func() time.Time {
		queueRequests.Add(1)
		return time.Now()
	})
	defer manager.Close()
	slotHolder := holdTheOnlySlot(t, manager)
	const id = "transfer_waiting_slot"
	createRemoteJob(t, manager, id, sftp.RemoteCopy)

	time.Sleep(50 * time.Millisecond)
	before := queueRequests.Load()
	time.Sleep(600 * time.Millisecond)
	if waiting := queueRequests.Load() - before; waiting != 0 {
		t.Fatalf("the waiting job made %d queue requests while no slot opened", waiting)
	}

	if _, err := manager.UpdateJobFromClient(slotHolder, sftp.UpdateTransferJob{Action: sftp.TransferCancelAction}); err != nil {
		t.Fatal(err)
	}
	waitForJob(t, manager, id, hasStatus(sftp.TransferCompleted))
}

func TestAWaitingRemoteJobStartsWhenProcessingResumes(t *testing.T) {
	source := remoteWith(map[string]node{"/source.bin": file("source.bin", "payload", 0o644)})
	manager := newCopyJobManager(t, source, remoteWith(nil))
	defer manager.Close()
	stop := func(stopped bool) {
		t.Helper()
		settings := sftp.DefaultTransferSettings()
		settings.ProcessingStopped = stopped
		if err := manager.SetTransferSettings(settings); err != nil {
			t.Fatal(err)
		}
	}
	stop(true)
	const id = "transfer_stopped_queue"
	createRemoteJob(t, manager, id, sftp.RemoteCopy)
	time.Sleep(50 * time.Millisecond)
	if job := waitForJob(t, manager, id, func(sftp.TransferJob) bool { return true }); job.Status != sftp.TransferQueued {
		t.Fatalf("status while processing is stopped = %s, want queued", job.Status)
	}

	stop(false)
	waitForJob(t, manager, id, hasStatus(sftp.TransferCompleted))
}
