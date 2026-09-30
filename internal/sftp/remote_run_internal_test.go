package sftp

import (
	"context"
	"errors"
	"testing"
)

// The test plays the failed worker by hand, so the retry lands exactly after
// the failure is recorded and before that worker unregisters its run. Waiting
// on a real worker would almost always let it unregister first, so the job is
// admitted without the worker CreateJob would start.
func TestRetryBeforeTheFailedWorkerUnregistersStartsANewRun(t *testing.T) {
	offline := errors.New("the host is offline")
	manager := NewTransferManager(&Service{Open: func(context.Context, string) (Remote, error) {
		return nil, offline
	}}, t.TempDir())
	defer manager.Close()
	const id = "transfer_retry_window"
	if _, err := manager.admitJob(CreateTransferJob{
		ID: id, BatchID: "batch_" + id, Alias: "target", RemotePath: "/copy.bin",
		SourceAlias: "source", SourcePath: "/source.bin", Operation: RemoteCopy,
		Direction: TransferRemote, Kind: TransferFile, Name: "source.bin", TotalBytes: -1,
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	failedRun := &remoteRun{ctx: ctx, cancel: cancel}
	manager.remoteJobsMutex.Lock()
	manager.remoteRuns[id] = failedRun
	manager.remoteJobsMutex.Unlock()
	if _, err := manager.reportRemoteRun(failedRun, id, UpdateTransferJob{Action: TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.reportRemoteRun(failedRun, id, UpdateTransferJob{Action: TransferFailAction, Problem: "sftp_failed"}); err != nil {
		t.Fatal(err)
	}

	if _, err := manager.UpdateJobFromClient(id, UpdateTransferJob{Action: TransferRetryAction}); err != nil {
		t.Fatal(err)
	}
	manager.remoteJobsMutex.Lock()
	current := manager.remoteRuns[id]
	manager.remoteJobsMutex.Unlock()
	if current == failedRun {
		t.Fatal("the retry found the failed run still registered and started no worker")
	}
	// The failed worker leaves only now, and must not unregister the new run.
	manager.finishRemoteWorker(id, failedRun)
}
