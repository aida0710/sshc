package sftp_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"testing"

	"sshc/internal/sftp"
)

// rangedFake is the test remote with the range reads a split download uses.
type rangedFake struct {
	*fakeRemote
}

func (remote *rangedFake) OpenRange(candidate string, offset int64) (io.ReadCloser, error) {
	info, ok := remote.nodes[candidate]
	if !ok {
		return nil, fs.ErrNotExist
	}
	if offset > int64(len(info.content)) {
		return nil, fs.ErrInvalid
	}
	return io.NopCloser(bytes.NewReader(info.content[offset:])), nil
}

// A file just over the smallest split threshold, so a download asks for ranges.
func largeDownloadFixture() ([]byte, *fakeRemote) {
	contents := bytes.Repeat([]byte("0123456789abcdef"), int((sftp.MinLargeFileThreshold+1<<20)/16))
	return contents, remoteWith(map[string]node{"/large.bin": file("large.bin", string(contents), 0o644)})
}

func splitDownloadJob(t *testing.T, manager *sftp.TransferManager, contents []byte, parallelism int) sftp.TransferJob {
	t.Helper()
	job, err := manager.CreateJob(sftp.CreateTransferJob{
		ID: "transfer_split0001", BatchID: "batch_split0001", Alias: "edge", Direction: sftp.TransferDownload,
		Kind: sftp.TransferFile, Name: "large.bin", RemotePath: "/large.bin", TotalBytes: int64(len(contents)),
		LargeFileThresholdBytes: sftp.MinLargeFileThreshold, LargeFileParallelism: parallelism, LargeFileChunkBytes: sftp.MinLargeFileChunkBytes,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob(job.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	return job
}

func downloadedBytes(t *testing.T, prepared *sftp.PreparedDownload) []byte {
	t.Helper()
	var downloaded bytes.Buffer
	if _, err := prepared.WriteFrom(context.Background(), 0, &downloaded); err != nil {
		t.Fatal(err)
	}
	return downloaded.Bytes()
}

func TestASplitDownloadFallsBackToOneConnectionWhenTheHostRefusesMore(t *testing.T) {
	contents, remote := largeDownloadFixture()
	opens := 0
	manager := sftp.NewTransferManager(&sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) {
		opens++
		if opens > 1 {
			// The second code in a one-time-code window, or a per-user
			// connection cap: the host takes one connection, not four.
			return nil, errors.New("keyboard-interactive authentication failed")
		}
		return &rangedFake{fakeRemote: remote}, nil
	}})
	t.Cleanup(func() { _ = manager.Close() })
	job := splitDownloadJob(t, manager, contents, 4)

	prepared, err := manager.PrepareOwnedDownload(t.Context(), job.ID, job.Alias, job.RemotePath)
	if err != nil {
		t.Fatalf("PrepareOwnedDownload() = %v, want the download to continue over the open connection", err)
	}
	defer prepared.Close()
	if got := downloadedBytes(t, prepared); !bytes.Equal(got, contents) {
		t.Fatalf("downloaded %d bytes, want %d identical bytes", len(got), len(contents))
	}
	if opens != 2 {
		t.Fatalf("opened %d connections, want the first and one refused attempt", opens)
	}
}

func TestAHostLimitedToOneConnectionIsNeverAskedForMore(t *testing.T) {
	contents, remote := largeDownloadFixture()
	opens := 0
	manager := sftp.NewTransferManager(&sftp.Service{
		Open: func(context.Context, string) (sftp.Remote, error) {
			opens++
			return &rangedFake{fakeRemote: remote}, nil
		},
		ConnectionLimit: func(alias string) int { return 1 },
	})
	t.Cleanup(func() { _ = manager.Close() })
	job := splitDownloadJob(t, manager, contents, 4)

	prepared, err := manager.PrepareOwnedDownload(t.Context(), job.ID, job.Alias, job.RemotePath)
	if err != nil {
		t.Fatalf("PrepareOwnedDownload() = %v", err)
	}
	defer prepared.Close()
	if got := downloadedBytes(t, prepared); !bytes.Equal(got, contents) {
		t.Fatalf("downloaded %d bytes, want %d identical bytes", len(got), len(contents))
	}
	if opens != 1 {
		t.Fatalf("opened %d connections to a host that allows one", opens)
	}
}
