package sftp_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshc/internal/sftp"
)

// spoolDirectories lists the per-manager directories under a spool root. The
// quota lock beside them stays and is not counted.
func spoolDirectories(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var directories []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "sshc-sftp-spool-") {
			directories = append(directories, entry.Name())
		}
	}
	return directories
}

func startDownloadJob(t *testing.T, manager *sftp.TransferManager, id string) sftp.CreateTransferJob {
	t.Helper()
	input := sftp.CreateTransferJob{
		ID: id, BatchID: "batch_" + id, Alias: "edge", Direction: sftp.TransferDownload,
		Kind: sftp.TransferFile, Name: "small.bin", RemotePath: "/small.bin", TotalBytes: 4,
	}
	if _, err := manager.CreateJob(input); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob(input.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	return input
}

func TestAManagerThatNeverDownloadsLeavesNothingInTheSpoolRoot(t *testing.T) {
	root := t.TempDir()
	manager := sftp.NewTransferManager(nil, root)
	defer manager.Close()
	if err := manager.DownloadSpoolError(); err != nil {
		t.Fatalf("DownloadSpoolError() = %v", err)
	}
	if directories := spoolDirectories(t, root); len(directories) != 0 {
		t.Fatalf("spool directories before any download = %v", directories)
	}
}

func TestClosingTheManagerRemovesItsSpoolDirectory(t *testing.T) {
	remote := remoteWith(map[string]node{"/small.bin": file("small.bin", "data", 0o600)})
	root := t.TempDir()
	manager := sftp.NewTransferManager(&sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) {
		return remote, nil
	}}, root)
	defer manager.Close()
	input := startDownloadJob(t, manager, "transfer_spoolroot1")
	prepared, err := manager.PrepareOwnedDownload(t.Context(), input.ID, input.Alias, input.RemotePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.Close(); err != nil {
		t.Fatal(err)
	}
	if directories := spoolDirectories(t, root); len(directories) != 1 {
		t.Fatalf("spool directories during the download = %v, want one", directories)
	}

	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	if directories := spoolDirectories(t, root); len(directories) != 0 {
		t.Fatalf("spool directories after Close = %v", directories)
	}
}

func TestAnUnusableSpoolRootFailsDownloadsWithItsCause(t *testing.T) {
	notADirectory := filepath.Join(t.TempDir(), "sftp-spool")
	if err := os.WriteFile(notADirectory, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		root string
	}{
		{name: "no root", root: ""},
		{name: "a file in place of the root", root: notADirectory},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			remote := remoteWith(map[string]node{"/small.bin": file("small.bin", "data", 0o600)})
			manager := sftp.NewTransferManager(&sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) {
				return remote, nil
			}}, test.root)
			defer manager.Close()
			cause := manager.DownloadSpoolError()
			if cause == nil {
				t.Fatal("DownloadSpoolError() = nil for a root that cannot hold downloads")
			}
			input := startDownloadJob(t, manager, "transfer_spoolroot2")
			_, err := manager.PrepareOwnedDownload(t.Context(), input.ID, input.Alias, input.RemotePath)
			if !errors.Is(err, sftp.ErrSpoolUnavailable) || !errors.Is(err, cause) {
				t.Fatalf("PrepareOwnedDownload() = %v, want %v caused by %v", err, sftp.ErrSpoolUnavailable, cause)
			}
		})
	}
}

// preparedSpoolFiles lists the prepared download files in the per-manager
// directories under a spool root.
func preparedSpoolFiles(t *testing.T, root string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(root, "sshc-sftp-spool-*", "download-*.part"))
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestAFailedDownloadEvictedToAdmitANewJobLeavesNoSpoolFile(t *testing.T) {
	remote := remoteWith(map[string]node{"/small.bin": file("small.bin", "data", 0o600)})
	root := t.TempDir()
	manager := sftp.NewTransferManager(&sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) {
		return remote, nil
	}}, root)
	defer manager.Close()
	failed := startDownloadJob(t, manager, "transfer_evicted1")
	prepared, err := manager.PrepareOwnedDownload(t.Context(), failed.ID, failed.Alias, failed.RemotePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJobFromClient(failed.ID, sftp.UpdateTransferJob{Action: sftp.TransferFailAction, Problem: "network"}); err != nil {
		t.Fatal(err)
	}
	// A failed download keeps its spool, so that a retry sends the same bytes.
	if files := preparedSpoolFiles(t, root); len(files) != 1 {
		t.Fatalf("spool files after the failure = %v, want one", files)
	}
	for index := len(listJobs(t, manager)); index < 200; index++ {
		id := fmt.Sprintf("transfer_%08d", index)
		if _, err := manager.CreateJob(sftp.CreateTransferJob{
			ID: id, BatchID: "batch_" + id, Alias: "edge", Direction: sftp.TransferDownload,
			Kind: sftp.TransferFile, Name: id, RemotePath: "/" + id, TotalBytes: 1,
		}); err != nil {
			t.Fatalf("create %d: %v", index, err)
		}
	}

	if _, err := manager.CreateJob(sftp.CreateTransferJob{
		ID: "transfer_admitted", BatchID: "batch_admitted", Alias: "edge", Direction: sftp.TransferDownload,
		Kind: sftp.TransferFile, Name: "admitted", RemotePath: "/admitted", TotalBytes: 1,
	}); err != nil {
		t.Fatalf("admitting a job over a full queue = %v", err)
	}
	for _, job := range listJobs(t, manager) {
		if job.ID == failed.ID {
			t.Fatal("the failed download was not the job evicted for admission")
		}
	}
	if files := preparedSpoolFiles(t, root); len(files) != 0 {
		t.Fatalf("spool files after the failed download was evicted = %v", files)
	}
}
