package sftp_test

import (
	"testing"
	"time"

	"sshc/internal/sftp"
)

// 手動の消去と期限切れの消去は、終わった記録を同じ条件で外す。応答を送っている
// 途中の download のように data-plane の操作を持つ記録は、どちらの消去でも残る。
func TestFinishedRecordStaysUntilItsDataPlaneOperationEndsForManualAndAutomaticClearing(t *testing.T) {
	now := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	manager := newTestTransferManager(t, nil)
	manager.ConfigureJobs(1, func() time.Time { return now })
	settings := sftp.DefaultTransferSettings()
	settings.ClearCompletedAfter = sftp.MinClearCompletedAfter
	if err := manager.SetTransferSettings(settings); err != nil {
		t.Fatal(err)
	}
	input := sftp.CreateTransferJob{
		ID: "transfer_inuse01", BatchID: "batch_inuse0001", Alias: "edge", Direction: sftp.TransferDownload,
		Kind: sftp.TransferFile, Name: "file", RemotePath: "/file", TotalBytes: 6,
	}
	if _, err := manager.CreateJob(input); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob(input.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	_, done, err := manager.StartDownloadDataPlane(input.ID, input.Alias, input.RemotePath, input.Kind)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob(input.ID, sftp.UpdateTransferJob{Action: sftp.TransferCancelAction}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(sftp.MinClearCompletedAfter)

	if jobs := listJobs(t, manager); len(jobs) != 1 || jobs[0].Status != sftp.TransferCancelled {
		t.Fatalf("expiry removed a record whose download was still sending: %+v", jobs)
	}
	if removed, err := manager.ClearFinished(); err != nil || removed != 0 {
		t.Fatalf("ClearFinished = %d, %v; want the record kept while its download is sending", removed, err)
	}

	done()
	if removed, err := manager.ClearFinished(); err != nil || removed != 1 {
		t.Fatalf("ClearFinished after the download ended = %d, %v; want 1", removed, err)
	}
	if jobs := listJobs(t, manager); len(jobs) != 0 {
		t.Fatalf("remaining jobs = %+v", jobs)
	}
}
