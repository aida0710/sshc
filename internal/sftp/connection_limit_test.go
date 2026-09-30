package sftp_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"testing"

	"sshc/internal/sftp"
)

// limitedUpload is one large upload to a host whose connection limit a test
// changes between requests, as unlocking the vault or binding a one-time code
// does.
type limitedUpload struct {
	manager     *sftp.TransferManager
	remote      *fakeRemote
	job         sftp.CreateTransferJob
	contents    []byte
	fingerprint string
	limit       int
}

func newLimitedUpload(t *testing.T, limit int) *limitedUpload {
	t.Helper()
	contents := bytes.Repeat([]byte("one-connection\n."), int(sftp.MinLargeFileThreshold/16))
	upload := &limitedUpload{remote: remoteWith(nil), contents: contents, limit: limit}
	service := serviceFor(upload.remote)
	service.ConnectionLimit = func(string) int { return upload.limit }
	upload.manager = newTestTransferManager(t, &service)
	t.Cleanup(func() { _ = upload.manager.Close() })
	upload.job = sftp.CreateTransferJob{
		ID: "transfer_totp0001", BatchID: "batch_totp0001", Alias: "edge", Direction: sftp.TransferUpload,
		Kind: sftp.TransferFile, Name: "large.bin", RemotePath: "/large.bin", TotalBytes: int64(len(contents)),
		LargeFileThresholdBytes: sftp.MinLargeFileThreshold, LargeFileParallelism: 4, LargeFileChunkBytes: sftp.MinLargeFileChunkBytes,
	}
	if _, err := upload.manager.CreateJob(upload.job); err != nil {
		t.Fatal(err)
	}
	upload.update(t, sftp.TransferStartAction)
	fingerprint, err := sftp.SourceFingerprint(t.Context(), bytes.NewReader(contents), upload.job.TotalBytes)
	if err != nil {
		t.Fatal(err)
	}
	upload.fingerprint = fingerprint
	return upload
}

func (u *limitedUpload) update(t *testing.T, action sftp.TransferJobAction) {
	t.Helper()
	if _, err := u.manager.UpdateJob(u.job.ID, sftp.UpdateTransferJob{Action: action}); err != nil {
		t.Fatalf("%s: %v", action, err)
	}
}

func (u *limitedUpload) start(t *testing.T) sftp.ResumableUpload {
	t.Helper()
	started, err := u.manager.StartOwned(t.Context(), u.job.Alias, u.job.ID, u.job.RemotePath, sftp.StartUploadOptions{
		Size: u.job.TotalBytes, SourceFingerprint: u.fingerprint,
	})
	if err != nil {
		t.Fatal(err)
	}
	return started
}

func (u *limitedUpload) sendRange(t *testing.T, offset int64) error {
	t.Helper()
	size := min(u.job.LargeFileChunkBytes, u.job.TotalBytes-offset)
	_, err := u.manager.AppendRangeOwned(t.Context(), sftp.UploadRangeWrite{
		Target: sftp.UploadTarget{Alias: u.job.Alias, ID: u.job.ID, RemotePath: u.job.RemotePath},
		Range:  sftp.UploadRange{Offset: offset, Size: size}, Total: u.job.TotalBytes,
		Contents: bytes.NewReader(u.contents[offset : offset+size]),
	})
	return err
}

func (u *limitedUpload) sendRestAsOneStream(t *testing.T, offset int64) {
	t.Helper()
	if _, err := u.manager.AppendOwned(t.Context(), sftp.UploadAppend{Target: sftp.UploadTarget{Alias: u.job.Alias, ID: u.job.ID, RemotePath: u.job.RemotePath}, Offset: offset, Total: u.job.TotalBytes, Contents: u.contents[offset:]}); err != nil {
		t.Fatal(err)
	}
}

func (u *limitedUpload) complete(t *testing.T, started sftp.ResumableUpload) {
	t.Helper()
	if _, err := u.manager.CompleteOwned(t.Context(), sftp.UploadCompletion{Target: sftp.UploadTarget{Alias: u.job.Alias, ID: u.job.ID, RemotePath: u.job.RemotePath}, Total: u.job.TotalBytes, ExpectedRevision: started.ExpectedRevision, SourceFingerprint: u.fingerprint}); err != nil {
		t.Fatalf("complete = %v", err)
	}
	if !bytes.Equal(u.remote.nodes[u.job.RemotePath].content, u.contents) {
		t.Fatal("uploaded contents differ")
	}
}

func TestSplitUploadToAHostLimitedToOneConnectionRunsAsOneStream(t *testing.T) {
	upload := newLimitedUpload(t, 1)

	started := upload.start(t)
	if started.Parallelism != 1 {
		t.Fatalf("parallelism = %d, want one stream to a host that allows one connection", started.Parallelism)
	}
	if err := upload.sendRange(t, 0); !errors.Is(err, sftp.ErrInvalidTransfer) {
		t.Fatalf("range upload = %v, want ErrInvalidTransfer", err)
	}
	upload.sendRestAsOneStream(t, 0)
	upload.complete(t, started)
}

// A split upload prepares its part at the final size, so the bytes between
// its ranges are holes. Resumed as one stream once the host allows one
// connection, it keeps only the ranges that lead the file and sends the rest.
func TestASplitUploadResumedAfterTheHostAllowsOneConnectionCompletesAsOneStream(t *testing.T) {
	chunk := sftp.MinLargeFileChunkBytes
	for _, test := range []struct {
		name       string
		sentRange  int64
		wantOffset int64
	}{
		{name: "the leading range was sent", sentRange: 0, wantOffset: chunk},
		{name: "only a later range was sent", sentRange: chunk, wantOffset: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			upload := newLimitedUpload(t, 0)
			if started := upload.start(t); started.Parallelism < 2 {
				t.Fatalf("parallelism = %d, want ranges while the host has no limit", started.Parallelism)
			}
			if err := upload.sendRange(t, test.sentRange); err != nil {
				t.Fatal(err)
			}
			upload.update(t, sftp.TransferPauseAction)
			upload.limit = 1
			upload.update(t, sftp.TransferResumeAction)
			upload.update(t, sftp.TransferStartAction)

			resumed := upload.start(t)
			if resumed.Parallelism != 1 || resumed.Offset != test.wantOffset {
				t.Fatalf("resume = parallelism %d offset %d, want one stream from %d", resumed.Parallelism, resumed.Offset, test.wantOffset)
			}
			upload.sendRestAsOneStream(t, resumed.Offset)
			upload.complete(t, resumed)
		})
	}
}

func TestRangesStillInFlightWhenTheHostLimitDropsAreAccepted(t *testing.T) {
	upload := newLimitedUpload(t, 0)
	started := upload.start(t)
	if started.Parallelism < 2 {
		t.Fatalf("parallelism = %d, want ranges while the host has no limit", started.Parallelism)
	}
	upload.limit = 1

	for offset := int64(0); offset < upload.job.TotalBytes; offset += upload.job.LargeFileChunkBytes {
		if err := upload.sendRange(t, offset); err != nil {
			t.Fatalf("range at %d = %v", offset, err)
		}
	}
	upload.complete(t, started)
}

func TestCopyAndMoveWithinOneHostOpenOneConnection(t *testing.T) {
	for _, operation := range []sftp.RemoteTransferOperation{sftp.RemoteCopy, sftp.RemoteMove} {
		t.Run(string(operation), func(t *testing.T) {
			remote := remoteWith(map[string]node{
				"/data":          {name: "data", mode: fs.ModeDir | 0o755, modTime: testTime},
				"/data/file.txt": {name: "file.txt", mode: 0o640, content: []byte("same host"), modTime: testTime},
				"/inbox":         {name: "inbox", mode: fs.ModeDir | 0o755, modTime: testTime},
			})
			opens := 0
			service := serviceFor(remote)
			service.Open = func(context.Context, string) (sftp.Remote, error) {
				opens++
				return remote, nil
			}

			err := service.CopyRemote(t.Context(), sftp.RemoteTransferRequest{
				SourceAlias: "edge", SourcePath: "/data/file.txt",
				TargetAlias: "edge", TargetPath: "/inbox/file.txt", Operation: operation,
			}, func(int64) error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			if opens != 1 {
				t.Fatalf("opened %d connections, want one for both ends", opens)
			}
			if got := string(remote.nodes["/inbox/file.txt"].content); got != "same host" {
				t.Fatalf("target contents = %q", got)
			}
		})
	}
}
