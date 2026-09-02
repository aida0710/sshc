package sftp_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"time"

	"sshc/internal/sftp"
)

func TestAllowedTransferActionsAreDerivedFromEngineState(t *testing.T) {
	tests := []struct {
		status sftp.TransferJobStatus
		want   []sftp.TransferControlAction
	}{
		{sftp.TransferQueued, []sftp.TransferControlAction{sftp.TransferPauseControl, sftp.TransferCancelControl}},
		{sftp.TransferRunning, []sftp.TransferControlAction{sftp.TransferPauseControl, sftp.TransferCancelControl}},
		{sftp.TransferPaused, []sftp.TransferControlAction{sftp.TransferResumeControl, sftp.TransferCancelControl}},
		{sftp.TransferReattach, []sftp.TransferControlAction{sftp.TransferResumeControl, sftp.TransferCancelControl}},
		{sftp.TransferNeedsOverwrite, []sftp.TransferControlAction{sftp.TransferResumeControl, sftp.TransferCancelControl}},
		{sftp.TransferFailed, []sftp.TransferControlAction{sftp.TransferRetryControl, sftp.TransferCancelControl}},
		{sftp.TransferCompleted, []sftp.TransferControlAction{}},
		{sftp.TransferCancelled, []sftp.TransferControlAction{}},
	}
	for _, test := range tests {
		t.Run(string(test.status), func(t *testing.T) {
			got := sftp.AllowedTransferActions(sftp.TransferJob{Status: test.status})
			if !slices.Equal(got, test.want) {
				t.Fatalf("actions = %v, want %v", got, test.want)
			}
		})
	}
}

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

func TestTransferLedgerOwnsBatchMetadataAndClearsOnlyFinishedJobs(t *testing.T) {
	manager := sftp.NewTransferManager(nil)
	completed, err := manager.CreateJob(sftp.CreateTransferJob{
		ID: "transfer_metadata", BatchID: "batch_metadata1", BatchName: "project", BatchKind: sftp.TransferFolder,
		Alias: "edge", Direction: sftp.TransferDownload, Kind: sftp.TransferFile, Name: "file.bin",
		RemotePath: "/project/file.bin", TotalBytes: 4, LastModified: 123,
	})
	if err != nil || completed.BatchName != "project" || completed.BatchKind != sftp.TransferFolder || completed.LastModified != 123 {
		t.Fatalf("created metadata = %+v, %v", completed, err)
	}
	if _, err := manager.UpdateJob(completed.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	progress := int64(4)
	if _, err := manager.UpdateJob(completed.ID, sftp.UpdateTransferJob{Action: sftp.TransferProgressAction, TransferredBytes: &progress}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob(completed.ID, sftp.UpdateTransferJob{Action: sftp.TransferCompleteAction}); err != nil {
		t.Fatal(err)
	}
	active, err := manager.CreateJob(sftp.CreateTransferJob{
		ID: "transfer_active1", BatchID: "batch_metadata1", BatchName: "project", BatchKind: sftp.TransferFolder,
		Alias: "edge", Direction: sftp.TransferDownload, Kind: sftp.TransferFile, Name: "active.bin",
		RemotePath: "/project/active.bin", TotalBytes: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if removed := manager.ClearFinished(); removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	jobs := manager.ListJobs()
	if len(jobs) != 1 || jobs[0].ID != active.ID {
		t.Fatalf("remaining jobs = %+v", jobs)
	}
}

func TestQueueReorderMovesOnlyWaitingJobs(t *testing.T) {
	now := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	manager := sftp.NewTransferManager(nil)
	manager.ConfigureJobs(1, func() time.Time { return now })
	for _, id := range []string{"transfer_first01", "transfer_second1", "transfer_third01"} {
		if _, err := manager.CreateJob(sftp.CreateTransferJob{
			ID: id, BatchID: "batch_reorder1", Alias: "edge", Direction: sftp.TransferDownload,
			Kind: sftp.TransferFile, Name: id + ".bin", RemotePath: "/" + id + ".bin", TotalBytes: 8,
		}); err != nil {
			t.Fatal(err)
		}
	}
	// 先頭を running にする。並べ替えても running は動かない。
	if _, err := manager.UpdateJob("transfer_first01", sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}

	if err := manager.MoveQueuedJob("transfer_third01", sftp.TransferMoveUp); err != nil {
		t.Fatal(err)
	}
	if order := jobIDs(manager); !slices.Equal(order, []string{"transfer_first01", "transfer_third01", "transfer_second1"}) {
		t.Fatalf("order after up = %v", order)
	}

	if err := manager.MoveQueuedJob("transfer_second1", sftp.TransferMoveTop); err != nil {
		t.Fatal(err)
	}
	if order := jobIDs(manager); !slices.Equal(order, []string{"transfer_first01", "transfer_second1", "transfer_third01"}) {
		t.Fatalf("order after top = %v", order)
	}

	if err := manager.MoveQueuedJob("transfer_first01", sftp.TransferMoveDown); !errors.Is(err, sftp.ErrTransferState) {
		t.Fatalf("moving a running job = %v, want %v", err, sftp.ErrTransferState)
	}
	if err := manager.MoveQueuedJob("transfer_second1", "sideways"); !errors.Is(err, sftp.ErrInvalidTransfer) {
		t.Fatalf("unknown move = %v, want %v", err, sftp.ErrInvalidTransfer)
	}
	if err := manager.MoveQueuedJob("transfer_missing1", sftp.TransferMoveUp); !errors.Is(err, sftp.ErrTransferNotFound) {
		t.Fatalf("unknown job = %v, want %v", err, sftp.ErrTransferNotFound)
	}
}

func TestTransferSettingsBoundConcurrencyAndExpireFinishedJobs(t *testing.T) {
	now := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	manager := sftp.NewTransferManager(nil)
	manager.ConfigureJobs(2, func() time.Time { return now })

	for _, invalid := range []struct {
		concurrency int
		clearAfter  time.Duration
	}{
		{concurrency: 0, clearAfter: 0},
		{concurrency: sftp.MaxTransferConcurrency + 1, clearAfter: 0},
		{concurrency: 2, clearAfter: time.Second},
		{concurrency: 2, clearAfter: sftp.MaxClearCompletedAfter + time.Second},
	} {
		if err := manager.SetTransferSettings(invalid.concurrency, invalid.clearAfter, false); !errors.Is(err, sftp.ErrInvalidTransfer) {
			t.Fatalf("SetTransferSettings(%d, %v) = %v", invalid.concurrency, invalid.clearAfter, err)
		}
	}
	if err := manager.SetTransferSettings(4, time.Minute, false); err != nil {
		t.Fatal(err)
	}
	if manager.MaxConcurrent() != 4 || manager.ClearCompletedAfter() != time.Minute {
		t.Fatalf("settings = %d, %v", manager.MaxConcurrent(), manager.ClearCompletedAfter())
	}

	if _, err := manager.CreateJob(sftp.CreateTransferJob{
		ID: "transfer_expire01", BatchID: "batch_expire01", Alias: "edge", Direction: sftp.TransferDownload,
		Kind: sftp.TransferFile, Name: "done.bin", RemotePath: "/done.bin", TotalBytes: 4,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.CreateJob(sftp.CreateTransferJob{
		ID: "transfer_failed01", BatchID: "batch_expire01", Alias: "edge", Direction: sftp.TransferDownload,
		Kind: sftp.TransferFile, Name: "bad.bin", RemotePath: "/bad.bin", TotalBytes: 4,
	}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"transfer_expire01", "transfer_failed01"} {
		if _, err := manager.UpdateJob(id, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
			t.Fatal(err)
		}
	}
	progress := int64(4)
	if _, err := manager.UpdateJob("transfer_expire01", sftp.UpdateTransferJob{Action: sftp.TransferProgressAction, TransferredBytes: &progress}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob("transfer_expire01", sftp.UpdateTransferJob{Action: sftp.TransferCompleteAction}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob("transfer_failed01", sftp.UpdateTransferJob{Action: sftp.TransferFailAction, Problem: "sftp_failed"}); err != nil {
		t.Fatal(err)
	}
	if order := jobIDs(manager); !slices.Equal(order, []string{"transfer_expire01", "transfer_failed01"}) {
		t.Fatalf("order before expiry = %v", order)
	}

	now = now.Add(time.Minute)
	// 完了だけが消える。失敗は原因を読む前に消えてはならない。
	if order := jobIDs(manager); !slices.Equal(order, []string{"transfer_failed01"}) {
		t.Fatalf("order after expiry = %v", order)
	}
}

func TestStoppedQueueLeavesWaitingJobsWaiting(t *testing.T) {
	manager := sftp.NewTransferManager(nil)
	manager.ConfigureJobs(2, nil)
	for _, id := range []string{"transfer_running1", "transfer_waiting1"} {
		if _, err := manager.CreateJob(sftp.CreateTransferJob{
			ID: id, BatchID: "batch_stopped1", Alias: "edge", Direction: sftp.TransferDownload,
			Kind: sftp.TransferFile, Name: id + ".bin", RemotePath: "/" + id + ".bin", TotalBytes: 8,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := manager.UpdateJob("transfer_running1", sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	if err := manager.SetTransferSettings(2, 0, true); err != nil {
		t.Fatal(err)
	}
	if !manager.ProcessingStopped() {
		t.Fatal("ProcessingStopped() = false after stopping the queue")
	}

	// 停止中は新しく始まらない。
	if _, err := manager.UpdateJob("transfer_waiting1", sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); !errors.Is(err, sftp.ErrTransferLimit) {
		t.Fatalf("start while stopped = %v, want %v", err, sftp.ErrTransferLimit)
	}
	// すでに走っているものは止めない。止めたいなら pause がある。
	jobs := manager.ListJobs()
	if len(jobs) != 2 || jobs[0].Status != sftp.TransferRunning || jobs[1].Status != sftp.TransferQueued {
		t.Fatalf("jobs = %+v", jobs)
	}

	if err := manager.SetTransferSettings(2, 0, false); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob("transfer_waiting1", sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatalf("start after resuming = %v", err)
	}
}

func jobIDs(manager *sftp.TransferManager) []string {
	jobs := manager.ListJobs()
	ids := make([]string, 0, len(jobs))
	for _, job := range jobs {
		ids = append(ids, job.ID)
	}
	return ids
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

func TestActiveDataPlaneIsNotFailedByTheStaleSweep(t *testing.T) {
	now := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	manager := sftp.NewTransferManager(nil)
	manager.ConfigureJobs(1, func() time.Time { return now })
	created, err := manager.CreateJob(sftp.CreateTransferJob{
		ID: "transfer_longrun", BatchID: "batch_longrun1", Alias: "edge", Direction: sftp.TransferDownload,
		Kind: sftp.TransferFile, Name: "large", RemotePath: "/large", TotalBytes: 1 << 30,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob(created.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	done, err := manager.KeepJobActive(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(10 * time.Minute)
	if got := manager.ListJobs()[0].Status; got != sftp.TransferRunning {
		t.Fatalf("active data plane was swept as %q", got)
	}
	done()
	now = now.Add(3 * time.Minute)
	if got := manager.ListJobs()[0].Status; got != sftp.TransferFailed {
		t.Fatalf("inactive stale job status = %q", got)
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

func TestUploadDataPlaneRequiresTheRunningOwningJob(t *testing.T) {
	manager := sftp.NewTransferManager(nil)
	input := sftp.CreateTransferJob{
		ID: "transfer_owner01", BatchID: "batch_owner001", Alias: "edge", Direction: sftp.TransferUpload,
		Kind: sftp.TransferFile, Name: "file", RemotePath: "/file", TotalBytes: 6,
	}
	if _, err := manager.CreateJob(input); err != nil {
		t.Fatal(err)
	}
	if err := manager.AuthorizeUpload(input.ID, input.Alias, input.RemotePath, input.TotalBytes, false); !errors.Is(err, sftp.ErrTransferState) {
		t.Fatalf("queued authorization = %v", err)
	}
	if _, err := manager.UpdateJob(input.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	if err := manager.AuthorizeUpload(input.ID, input.Alias, input.RemotePath, input.TotalBytes, false); err != nil {
		t.Fatalf("owner authorization = %v", err)
	}
	for _, changed := range []struct {
		alias string
		path  string
		total int64
	}{{"other", "/file", 6}, {"edge", "/other", 6}, {"edge", "/file", 7}} {
		if err := manager.AuthorizeUpload(input.ID, changed.alias, changed.path, changed.total, false); !errors.Is(err, sftp.ErrConflict) {
			t.Fatalf("changed identity authorization = %v", err)
		}
	}
}

func TestRunningDownloadCanResetToAReplacementRevisionSize(t *testing.T) {
	manager := sftp.NewTransferManager(nil)
	created, err := manager.CreateJob(sftp.CreateTransferJob{
		ID: "transfer_replace", BatchID: "batch_replace01", Alias: "edge", Direction: sftp.TransferDownload,
		Kind: sftp.TransferFile, Name: "file", RemotePath: "/file", TotalBytes: 6,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob(created.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	oldProgress := int64(3)
	if _, err := manager.UpdateJob(created.ID, sftp.UpdateTransferJob{Action: sftp.TransferProgressAction, TransferredBytes: &oldProgress}); err != nil {
		t.Fatal(err)
	}
	zero, replacementSize := int64(0), int64(7)
	reset, err := manager.UpdateJob(created.ID, sftp.UpdateTransferJob{
		Action: sftp.TransferProgressAction, TransferredBytes: &zero, TotalBytes: &replacementSize, ResetProgress: true,
	})
	if err != nil || reset.TransferredBytes != 0 || reset.TotalBytes != replacementSize || reset.Status != sftp.TransferRunning {
		t.Fatalf("replacement reset = %+v, %v", reset, err)
	}
	const revision = `"content-sha256:replacement"`
	if _, err := manager.BeginDownload(created.ID, replacementSize, revision, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.RecordDownloadSent(created.ID, replacementSize, replacementSize, revision); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.AcknowledgeDownload(created.ID, replacementSize, revision); err != nil {
		t.Fatal(err)
	}
	completed, err := manager.UpdateJobFromClient(created.ID, sftp.UpdateTransferJob{Action: sftp.TransferCompleteAction})
	if err != nil || completed.Status != sftp.TransferCompleted || completed.TotalBytes != replacementSize {
		t.Fatalf("replacement completion = %+v, %v", completed, err)
	}
}

func TestDownloadDataPlaneRequiresTheRunningOwningJob(t *testing.T) {
	manager := sftp.NewTransferManager(nil)
	input := sftp.CreateTransferJob{
		ID: "transfer_downown", BatchID: "batch_downowner", Alias: "edge", Direction: sftp.TransferDownload,
		Kind: sftp.TransferFile, Name: "file", RemotePath: "/file", TotalBytes: 6,
	}
	if _, err := manager.CreateJob(input); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.AuthorizeDownload(input.ID, input.Alias, input.RemotePath, input.Kind); !errors.Is(err, sftp.ErrTransferState) {
		t.Fatalf("queued download authorization = %v", err)
	}
	if _, err := manager.UpdateJob(input.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	for _, changed := range []struct {
		id, alias, path string
		kind            sftp.TransferKind
	}{
		{"transfer_missing", "edge", "/file", sftp.TransferFile},
		{input.ID, "other", "/file", sftp.TransferFile},
		{input.ID, "edge", "/other", sftp.TransferFile},
		{input.ID, "edge", "/file", sftp.TransferFolder},
	} {
		if _, err := manager.AuthorizeDownload(changed.id, changed.alias, changed.path, changed.kind); err == nil {
			t.Fatalf("changed download identity was authorized: %+v", changed)
		}
	}
	if _, err := manager.AuthorizeDownload(input.ID, input.Alias, input.RemotePath, input.Kind); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJobFromClient(input.ID, sftp.UpdateTransferJob{Action: sftp.TransferCompleteAction}); !errors.Is(err, sftp.ErrTransferState) {
		t.Fatalf("early client completion = %v", err)
	}
	const revision = `"content-sha256:owner"`
	if _, err := manager.BeginDownload(input.ID, 6, revision, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.RecordDownloadSent(input.ID, 6, 6, revision); err != nil {
		t.Fatal(err)
	}
	if job, err := manager.AcknowledgeDownload(input.ID, 6, revision); err != nil || job.Status != sftp.TransferRunning {
		t.Fatalf("durable server-bounded progress = %+v, %v", job, err)
	}
	completed, err := manager.UpdateJobFromClient(input.ID, sftp.UpdateTransferJob{Action: sftp.TransferCompleteAction})
	if err != nil || completed.Status != sftp.TransferCompleted {
		t.Fatalf("durable client acknowledgement = %+v, %v", completed, err)
	}
}

func TestClientCannotForgeDownloadProgressThroughFailRetryAndComplete(t *testing.T) {
	manager := sftp.NewTransferManager(nil)
	input := sftp.CreateTransferJob{
		ID: "transfer_forgery", BatchID: "batch_forgery1", Alias: "edge", Direction: sftp.TransferDownload,
		Kind: sftp.TransferFile, Name: "file", RemotePath: "/file", TotalBytes: 6,
	}
	if _, err := manager.CreateJob(input); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJobFromClient(input.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	forged := int64(6)
	if _, err := manager.UpdateJobFromClient(input.ID, sftp.UpdateTransferJob{
		Action: sftp.TransferFailAction, TransferredBytes: &forged, Problem: "network",
	}); !errors.Is(err, sftp.ErrInvalidTransfer) {
		t.Fatalf("client fail progress = %v", err)
	}
	job := manager.ListJobs()[0]
	if job.Status != sftp.TransferRunning || job.TransferredBytes != 0 {
		t.Fatalf("rejected mutation changed job: %+v", job)
	}
	if _, err := manager.UpdateJobFromClient(input.ID, sftp.UpdateTransferJob{Action: sftp.TransferCompleteAction, TransferredBytes: &forged}); !errors.Is(err, sftp.ErrTransferState) {
		t.Fatalf("forged complete = %v", err)
	}
}

func TestDownloadProgressRequiresRevisionBoundServerSentBytesAndAllowsDurableRollback(t *testing.T) {
	manager := sftp.NewTransferManager(nil)
	input := sftp.CreateTransferJob{
		ID: "transfer_durable", BatchID: "batch_durable1", Alias: "edge", Direction: sftp.TransferDownload,
		Kind: sftp.TransferFile, Name: "file", RemotePath: "/file", TotalBytes: 8,
	}
	if _, err := manager.CreateJob(input); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob(input.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	const revision = `"content-sha256:abc"`
	if _, err := manager.BeginDownload(input.ID, 8, revision, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.AcknowledgeDownload(input.ID, 1, revision); !errors.Is(err, sftp.ErrOffsetMismatch) {
		t.Fatalf("ack beyond sent = %v", err)
	}
	if _, err := manager.RecordDownloadSent(input.ID, 6, 8, revision); err != nil {
		t.Fatal(err)
	}
	acked, err := manager.AcknowledgeDownload(input.ID, 4, revision)
	if err != nil || acked.TransferredBytes != 4 {
		t.Fatalf("durable ack = %+v, %v", acked, err)
	}
	rolledBack, err := manager.AcknowledgeDownload(input.ID, 2, revision)
	if err != nil || rolledBack.TransferredBytes != 2 {
		t.Fatalf("durable rollback = %+v, %v", rolledBack, err)
	}
	if _, err := manager.BeginDownload(input.ID, 8, revision, 2); err != nil {
		t.Fatalf("resume from durable offset = %v", err)
	}
	if _, err := manager.AcknowledgeDownload(input.ID, 2, `"other"`); !errors.Is(err, sftp.ErrTransferState) {
		t.Fatalf("wrong revision ack = %v", err)
	}
}

func TestCompleteDownloadVerificationCannotManufactureSentBytes(t *testing.T) {
	manager := sftp.NewTransferManager(nil)
	input := sftp.CreateTransferJob{
		ID: "transfer_verifyno", BatchID: "batch_verifyno1", Alias: "edge", Direction: sftp.TransferDownload,
		Kind: sftp.TransferFile, Name: "file", RemotePath: "/file", TotalBytes: 8,
	}
	if _, err := manager.CreateJob(input); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob(input.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	const revision = `"content-sha256:verify"`
	if _, err := manager.BeginDownload(input.ID, 8, revision, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.VerifyDownloadComplete(input.ID, 8, revision); !errors.Is(err, sftp.ErrOffsetMismatch) {
		t.Fatalf("verify without sent evidence = %v", err)
	}
	if _, err := manager.RecordDownloadSent(input.ID, 8, 8, revision); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.VerifyDownloadComplete(input.ID, 8, revision); err != nil {
		t.Fatalf("verify after full send = %v", err)
	}
}

func TestTransferQueueHasAHardLimitWhenNothingCanBeEvicted(t *testing.T) {
	manager := sftp.NewTransferManager(nil)
	for index := 0; index < 200; index++ {
		id := fmt.Sprintf("transfer_%08d", index)
		if _, err := manager.CreateJob(sftp.CreateTransferJob{
			ID: id, BatchID: "batch_capacity1", Alias: "edge", Direction: sftp.TransferDownload,
			Kind: sftp.TransferFile, Name: id, RemotePath: "/" + id, TotalBytes: 1,
		}); err != nil {
			t.Fatalf("create %d: %v", index, err)
		}
	}
	if _, err := manager.CreateJob(sftp.CreateTransferJob{
		ID: "transfer_overflow", BatchID: "batch_capacity1", Alias: "edge", Direction: sftp.TransferDownload,
		Kind: sftp.TransferFile, Name: "overflow", RemotePath: "/overflow", TotalBytes: 1,
	}); !errors.Is(err, sftp.ErrTransferLimit) {
		t.Fatalf("overflow = %v", err)
	}
}

func TestEvictingAFailedUploadCleansItsOrphanPart(t *testing.T) {
	remote := remoteWith(map[string]node{"/remote": directory("remote")})
	manager := sftp.NewTransferManager(&sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }})
	input := sftp.CreateTransferJob{
		ID: "transfer_orphan1", BatchID: "batch_orphan001", Alias: "edge", Direction: sftp.TransferUpload,
		Kind: sftp.TransferFile, Name: "orphan.bin", RemotePath: "/remote/orphan.bin", TotalBytes: 4,
	}
	if _, err := manager.CreateJob(input); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob(input.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	started, err := manager.Start(t.Context(), input.Alias, input.ID, input.RemotePath, sftp.StartUploadOptions{Size: 4})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Append(t.Context(), input.Alias, input.ID, input.RemotePath, 0, 4, []byte("part")); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob(input.ID, sftp.UpdateTransferJob{Action: sftp.TransferFailAction, Problem: "connection_lost"}); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 199; index++ {
		id := fmt.Sprintf("transfer_keep%03d", index)
		if _, err := manager.CreateJob(sftp.CreateTransferJob{
			ID: id, BatchID: "batch_orphan001", Alias: "edge", Direction: sftp.TransferDownload,
			Kind: sftp.TransferFile, Name: id, RemotePath: "/" + id, TotalBytes: 1,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := manager.CreateJob(sftp.CreateTransferJob{
		ID: "transfer_newslot", BatchID: "batch_orphan001", Alias: "edge", Direction: sftp.TransferDownload,
		Kind: sftp.TransferFile, Name: "new", RemotePath: "/new", TotalBytes: 1,
	}); err != nil {
		t.Fatal(err)
	}
	for candidate := range remote.nodes {
		if strings.Contains(candidate, started.ID) {
			t.Fatalf("evicted upload part remains at %q", candidate)
		}
	}
}

func TestFailedEvictionCleanupKeepsARetryableTombstone(t *testing.T) {
	remote := remoteWith(map[string]node{"/remote": directory("remote")})
	manager := sftp.NewTransferManager(&sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }})
	orphan := sftp.CreateTransferJob{
		ID: "transfer_tombstone", BatchID: "batch_tombstone", Alias: "edge", Direction: sftp.TransferUpload,
		Kind: sftp.TransferFile, Name: "orphan.bin", RemotePath: "/remote/orphan.bin", TotalBytes: 4,
	}
	if _, err := manager.CreateJob(orphan); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob(orphan.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Start(t.Context(), orphan.Alias, orphan.ID, orphan.RemotePath, sftp.StartUploadOptions{Size: 4}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Append(t.Context(), orphan.Alias, orphan.ID, orphan.RemotePath, 0, 4, []byte("part")); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob(orphan.ID, sftp.UpdateTransferJob{Action: sftp.TransferFailAction, Problem: "network"}); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 199; index++ {
		id := fmt.Sprintf("transfer_tomb%03d", index)
		if _, err := manager.CreateJob(sftp.CreateTransferJob{ID: id, BatchID: orphan.BatchID, Alias: "edge", Direction: sftp.TransferDownload, Kind: sftp.TransferFile, Name: id, RemotePath: "/" + id, TotalBytes: 1}); err != nil {
			t.Fatal(err)
		}
	}
	input := sftp.CreateTransferJob{ID: "transfer_aftertomb", BatchID: orphan.BatchID, Alias: "edge", Direction: sftp.TransferDownload, Kind: sftp.TransferFile, Name: "new", RemotePath: "/new", TotalBytes: 1}
	remote.removeErr = errors.New("cleanup unavailable")
	if _, err := manager.CreateJob(input); !errors.Is(err, sftp.ErrTransferLimit) {
		t.Fatalf("admission with failed cleanup = %v", err)
	}
	jobs := manager.ListJobs()
	if len(jobs) != 200 {
		t.Fatalf("jobs after failed cleanup = %d", len(jobs))
	}
	foundTombstone := false
	for _, job := range jobs {
		if job.ID == orphan.ID {
			foundTombstone = job.Problem == "sftp_cleanup_pending"
		}
		if job.ID == input.ID {
			t.Fatal("new job was admitted before orphan cleanup")
		}
	}
	if !foundTombstone {
		t.Fatal("cleanup tombstone was forgotten")
	}
	if _, err := manager.UpdateJobFromClient(orphan.ID, sftp.UpdateTransferJob{Action: sftp.TransferRetryAction}); !errors.Is(err, sftp.ErrTransferState) {
		t.Fatalf("client cleared cleanup tombstone = %v", err)
	}
	remote.removeErr = nil
	if _, err := manager.CreateJob(input); err != nil {
		t.Fatal(err)
	}
	for _, job := range manager.ListJobs() {
		if job.ID == orphan.ID {
			t.Fatal("cleanup tombstone remains after retry")
		}
	}
	part := "/remote/.orphan.bin.sshc-upload-transfer_tombstone.part"
	if _, err := remote.Lstat(part); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("orphan part remains after retry: %v", err)
	}
}

func TestFailedCleanupTombstoneDoesNotBlockSafeDownloadEviction(t *testing.T) {
	remote := remoteWith(map[string]node{"/remote": directory("remote")})
	manager := sftp.NewTransferManager(&sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }})
	orphan := sftp.CreateTransferJob{
		ID: "transfer_safeevict", BatchID: "batch_safeevict", Alias: "edge", Direction: sftp.TransferUpload,
		Kind: sftp.TransferFile, Name: "orphan.bin", RemotePath: "/remote/orphan.bin", TotalBytes: 4,
	}
	if _, err := manager.CreateJob(orphan); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob(orphan.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Start(t.Context(), orphan.Alias, orphan.ID, orphan.RemotePath, sftp.StartUploadOptions{Size: 4}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Append(t.Context(), orphan.Alias, orphan.ID, orphan.RemotePath, 0, 4, []byte("part")); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob(orphan.ID, sftp.UpdateTransferJob{Action: sftp.TransferFailAction, Problem: "network"}); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 199; index++ {
		id := fmt.Sprintf("transfer_safe%03d", index)
		if _, err := manager.CreateJob(sftp.CreateTransferJob{ID: id, BatchID: orphan.BatchID, Alias: "edge", Direction: sftp.TransferDownload, Kind: sftp.TransferFile, Name: id, RemotePath: "/" + id, TotalBytes: 1}); err != nil {
			t.Fatal(err)
		}
	}
	remote.removeErr = errors.New("cleanup unavailable")
	first := sftp.CreateTransferJob{ID: "transfer_blocked1", BatchID: orphan.BatchID, Alias: "edge", Direction: sftp.TransferDownload, Kind: sftp.TransferFile, Name: "blocked", RemotePath: "/blocked", TotalBytes: 1}
	if _, err := manager.CreateJob(first); !errors.Is(err, sftp.ErrTransferLimit) {
		t.Fatalf("initial failed cleanup = %v", err)
	}
	if _, err := manager.UpdateJobFromClient("transfer_safe000", sftp.UpdateTransferJob{Action: sftp.TransferCancelAction}); err != nil {
		t.Fatal(err)
	}
	removals := len(remote.removals)
	admitted := sftp.CreateTransferJob{ID: "transfer_admitted", BatchID: orphan.BatchID, Alias: "other", Direction: sftp.TransferDownload, Kind: sftp.TransferFile, Name: "admitted", RemotePath: "/admitted", TotalBytes: 1}
	if _, err := manager.CreateJob(admitted); err != nil {
		t.Fatalf("safe terminal eviction was blocked by tombstone: %v", err)
	}
	if len(remote.removals) != removals {
		t.Fatal("cleanup tombstone was retried before a network-free eviction")
	}
	jobs := manager.ListJobs()
	if len(jobs) != 200 {
		t.Fatalf("jobs after safe eviction = %d", len(jobs))
	}
	foundTombstone, foundAdmitted := false, false
	for _, job := range jobs {
		if job.ID == orphan.ID {
			foundTombstone = job.Problem == "sftp_cleanup_pending"
		}
		foundAdmitted = foundAdmitted || job.ID == admitted.ID
	}
	if !foundTombstone || !foundAdmitted {
		t.Fatalf("tombstone/admitted = %v/%v", foundTombstone, foundAdmitted)
	}
}

func TestEvictionNetworkCleanupDoesNotHoldTheJobsMutex(t *testing.T) {
	remote := remoteWith(map[string]node{"/remote": directory("remote")})
	cleanupStarted := make(chan struct{})
	releaseCleanup := make(chan struct{})
	remote.removeHook = func(string) {
		select {
		case <-cleanupStarted:
		default:
			close(cleanupStarted)
		}
		<-releaseCleanup
	}
	manager := sftp.NewTransferManager(&sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }})
	input := sftp.CreateTransferJob{
		ID: "transfer_evictio", BatchID: "batch_eviction1", Alias: "edge", Direction: sftp.TransferUpload,
		Kind: sftp.TransferFile, Name: "old", RemotePath: "/remote/old", TotalBytes: 1,
	}
	if _, err := manager.CreateJob(input); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob(input.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Start(t.Context(), input.Alias, input.ID, input.RemotePath, sftp.StartUploadOptions{Size: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob(input.ID, sftp.UpdateTransferJob{Action: sftp.TransferFailAction, Problem: "network"}); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 199; index++ {
		id := fmt.Sprintf("transfer_lock%03d", index)
		if _, err := manager.CreateJob(sftp.CreateTransferJob{ID: id, BatchID: "batch_eviction1", Alias: "edge", Direction: sftp.TransferDownload, Kind: sftp.TransferFile, Name: id, RemotePath: "/" + id, TotalBytes: 1}); err != nil {
			t.Fatal(err)
		}
	}
	created := make(chan error, 1)
	go func() {
		_, err := manager.CreateJob(sftp.CreateTransferJob{ID: "transfer_locknew", BatchID: "batch_eviction1", Alias: "edge", Direction: sftp.TransferDownload, Kind: sftp.TransferFile, Name: "new", RemotePath: "/new", TotalBytes: 1})
		created <- err
	}()
	<-cleanupStarted
	listed := make(chan struct{})
	go func() { _ = manager.ListJobs(); close(listed) }()
	select {
	case <-listed:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("ListJobs blocked on eviction network cleanup")
	}
	close(releaseCleanup)
	if err := <-created; err != nil {
		t.Fatal(err)
	}
}
