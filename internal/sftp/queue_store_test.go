package sftp_test

import (
	"path/filepath"
	"testing"

	"sshc/internal/sftp"
)

func TestTransferQueueRestoresAfterEngineRestart(t *testing.T) {
	t.Parallel()
	filename := filepath.Join(t.TempDir(), "sshc", "transfers.json")
	first := sftp.NewTransferManager(nil)
	if err := first.EnableQueuePersistence(filename); err != nil {
		t.Fatal(err)
	}
	created, err := first.CreateJob(sftp.CreateTransferJob{
		ID: "transfer_persist_01", BatchID: "batch_persist_01", Alias: "edge",
		Direction: sftp.TransferDownload, Kind: sftp.TransferFile, Name: "large.bin",
		RemotePath: "/large.bin", TotalBytes: 4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.UpdateJob(created.ID, sftp.UpdateTransferJob{Action: sftp.TransferPauseAction}); err != nil {
		t.Fatal(err)
	}
	second := sftp.NewTransferManager(nil)
	if err := second.EnableQueuePersistence(filename); err != nil {
		t.Fatal(err)
	}
	jobs := second.ListJobs()
	if len(jobs) != 1 || jobs[0].ID != created.ID || jobs[0].Status != sftp.TransferPaused {
		t.Fatalf("restored jobs = %#v", jobs)
	}
	info, err := filepath.Glob(filepath.Join(filepath.Dir(filename), ".transfers-*.tmp"))
	if err != nil || len(info) != 0 {
		t.Fatalf("temporary queue files = %v, %v", info, err)
	}
}
