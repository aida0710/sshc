//go:build !windows

package sftp_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"sshc/internal/sftp"
)

// lockedFolder makes a folder under a temporary home that the engine's user
// cannot open, as file permissions or macOS's privacy protection would.
func lockedFolder(t *testing.T) string {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root opens folders regardless of their permissions")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	locked := filepath.Join(home, "locked")
	if err := os.Mkdir(locked, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
	return locked
}

func TestListingAFolderTheEngineMayNotOpenSaysTheRefusalIsLocal(t *testing.T) {
	locked := lockedFolder(t)
	_, err := sftp.ListLocal(locked)
	if !errors.Is(err, sftp.ErrLocalPermissionDenied) {
		t.Fatalf("listing a locked folder = %v, want ErrLocalPermissionDenied", err)
	}
}

func TestADownloadIntoAFolderTheEngineMayNotOpenFailsWithTheLocalRefusal(t *testing.T) {
	locked := lockedFolder(t)
	remote := remoteWith(map[string]node{
		"/remote":            directory("remote"),
		"/remote/report.txt": file("report.txt", "report", 0o644),
	})
	service := &sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }}
	manager := newTestTransferManager(t, service)
	t.Cleanup(func() { _ = manager.Close() })
	if _, err := manager.CreateJob(sftp.CreateTransferJob{
		ID: "locked_get", BatchID: "locked_batch", Alias: "edge", SourceAlias: "edge", SourcePath: "/remote/report.txt",
		RemotePath: filepath.Join(locked, "report.txt"), Direction: sftp.TransferRemote, Operation: sftp.RemoteGet,
		Kind: sftp.TransferFile, Name: "report.txt", TotalBytes: -1,
	}); err != nil {
		t.Fatal(err)
	}
	job := waitForJob(t, manager, "locked_get", func(job sftp.TransferJob) bool {
		return job.Status == sftp.TransferFailed || job.Status == sftp.TransferCompleted
	})
	if job.Status != sftp.TransferFailed || job.Problem != "sftp_local_permission_denied" {
		t.Fatalf("job = %s/%s, want failed/sftp_local_permission_denied", job.Status, job.Problem)
	}
}
