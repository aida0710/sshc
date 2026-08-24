package sftp_test

import (
	"errors"
	"testing"
	"time"

	"sshc/internal/sftp"
)

func TestTransferJobsShareConcurrencyAndTrackRate(t *testing.T) {
	now := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	manager := sftp.NewTransferManager(nil)
	manager.ConfigureJobs(2, func() time.Time { return now })
	for _, input := range []sftp.CreateTransferJob{
		{ID: "transfer_upload1", BatchID: "batch_upload01", Alias: "edge", Direction: sftp.TransferUpload, Kind: sftp.TransferFile, Name: "a.bin", RemotePath: "/a.bin", TotalBytes: 1_000},
		{ID: "transfer_download", BatchID: "batch_download", Alias: "edge", Direction: sftp.TransferDownload, Kind: sftp.TransferFile, Name: "b.bin", RemotePath: "/b.bin", TotalBytes: 2_000},
		{ID: "transfer_folder1", BatchID: "batch_folder01", Alias: "edge", Direction: sftp.TransferDownload, Kind: sftp.TransferFolder, Name: "folder", RemotePath: "/folder", TotalBytes: -1},
	} {
		if _, err := manager.CreateJob(input); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := manager.UpdateJob("transfer_upload1", sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob("transfer_download", sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob("transfer_folder1", sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); !errors.Is(err, sftp.ErrTransferLimit) {
		t.Fatalf("third start = %v, want limit", err)
	}
	now = now.Add(time.Second)
	progress := int64(400)
	job, err := manager.UpdateJob("transfer_upload1", sftp.UpdateTransferJob{Action: sftp.TransferProgressAction, TransferredBytes: &progress})
	if err != nil {
		t.Fatal(err)
	}
	if job.BytesPerSecond != 400 || job.RemainingSeconds != 2 {
		t.Fatalf("rate = %f, eta = %d", job.BytesPerSecond, job.RemainingSeconds)
	}
	if _, err := manager.UpdateJob("transfer_upload1", sftp.UpdateTransferJob{Action: sftp.TransferPauseAction}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob("transfer_folder1", sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatalf("released slot was not reusable: %v", err)
	}
}

func TestTransferJobStateMachineRejectsRollbackAndRetriesOnlyFailedJob(t *testing.T) {
	now := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	manager := sftp.NewTransferManager(nil)
	manager.ConfigureJobs(1, func() time.Time { return now })
	created, err := manager.CreateJob(sftp.CreateTransferJob{
		ID: "transfer_retry1", BatchID: "batch_retry001", Alias: "edge", Direction: sftp.TransferUpload,
		Kind: sftp.TransferFile, Name: "failed.bin", RemotePath: "/failed.bin", TotalBytes: 10,
	})
	if err != nil || created.Status != sftp.TransferQueued || created.Attempt != 1 {
		t.Fatalf("created = %+v, %v", created, err)
	}
	if _, err := manager.UpdateJob(created.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	progress := int64(6)
	if _, err := manager.UpdateJob(created.ID, sftp.UpdateTransferJob{Action: sftp.TransferProgressAction, TransferredBytes: &progress}); err != nil {
		t.Fatal(err)
	}
	rollback := int64(5)
	if _, err := manager.UpdateJob(created.ID, sftp.UpdateTransferJob{Action: sftp.TransferProgressAction, TransferredBytes: &rollback}); !errors.Is(err, sftp.ErrOffsetMismatch) {
		t.Fatalf("rollback = %v", err)
	}
	failed, err := manager.UpdateJob(created.ID, sftp.UpdateTransferJob{Action: sftp.TransferFailAction, Problem: "connection_lost"})
	if err != nil || failed.Status != sftp.TransferFailed || failed.TransferredBytes != 6 {
		t.Fatalf("failed = %+v, %v", failed, err)
	}
	retried, err := manager.UpdateJob(created.ID, sftp.UpdateTransferJob{Action: sftp.TransferRetryAction})
	if err != nil || retried.Status != sftp.TransferQueued || retried.Attempt != 2 || retried.TransferredBytes != 6 {
		t.Fatalf("retried = %+v, %v", retried, err)
	}
	if _, err := manager.UpdateJob(created.ID, sftp.UpdateTransferJob{Action: sftp.TransferCompleteAction}); !errors.Is(err, sftp.ErrTransferState) {
		t.Fatalf("complete before running = %v", err)
	}
}

func TestTransferJobCreateIsIdempotentAndRejectsChangedIdentity(t *testing.T) {
	manager := sftp.NewTransferManager(nil)
	input := sftp.CreateTransferJob{
		ID: "transfer_same01", BatchID: "batch_same0001", Alias: "edge", Direction: sftp.TransferDownload,
		Kind: sftp.TransferFile, Name: "same", RemotePath: "/same", TotalBytes: 3,
	}
	first, err := manager.CreateJob(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.CreateJob(input)
	if err != nil || second.CreatedAt != first.CreatedAt {
		t.Fatalf("idempotent create = %+v, %v", second, err)
	}
	input.RemotePath = "/other"
	if _, err := manager.CreateJob(input); !errors.Is(err, sftp.ErrConflict) {
		t.Fatalf("changed identity = %v", err)
	}
}

func TestStaleRunningTransferReleasesItsSlot(t *testing.T) {
	now := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	manager := sftp.NewTransferManager(nil)
	manager.ConfigureJobs(1, func() time.Time { return now })
	for _, id := range []string{"transfer_stale1", "transfer_waiting"} {
		_, err := manager.CreateJob(sftp.CreateTransferJob{
			ID: id, BatchID: "batch_stale01", Alias: "edge", Direction: sftp.TransferUpload,
			Kind: sftp.TransferFile, Name: id, RemotePath: "/" + id, TotalBytes: 10,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := manager.UpdateJob("transfer_stale1", sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(3 * time.Minute)
	jobs := manager.ListJobs()
	if jobs[0].Status != sftp.TransferFailed || jobs[0].Problem != "transfer_interrupted" {
		t.Fatalf("stale job = %+v", jobs[0])
	}
	if _, err := manager.UpdateJob("transfer_waiting", sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatalf("stale slot was not released: %v", err)
	}
}

func TestTransferRetryCanResetNonResumableProgress(t *testing.T) {
	manager := sftp.NewTransferManager(nil)
	created, err := manager.CreateJob(sftp.CreateTransferJob{
		ID: "transfer_archive", BatchID: "batch_archive01", Alias: "edge", Direction: sftp.TransferDownload,
		Kind: sftp.TransferFolder, Name: "logs", RemotePath: "/logs", TotalBytes: -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob(created.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	progress := int64(4096)
	if _, err := manager.UpdateJob(created.ID, sftp.UpdateTransferJob{Action: sftp.TransferProgressAction, TransferredBytes: &progress}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob(created.ID, sftp.UpdateTransferJob{Action: sftp.TransferFailAction, Problem: "download_failed"}); err != nil {
		t.Fatal(err)
	}
	retried, err := manager.UpdateJob(created.ID, sftp.UpdateTransferJob{Action: sftp.TransferRetryAction, ResetProgress: true})
	if err != nil || retried.TransferredBytes != 0 || retried.Attempt != 2 || retried.Status != sftp.TransferQueued {
		t.Fatalf("reset retry = %+v, %v", retried, err)
	}
}
