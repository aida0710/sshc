package sftp_test

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"sshc/internal/sftp"
)

var errBrowserGone = errors.New("the browser closed the response")

// goneAfterFirstWrite takes the first write and refuses the rest, like a
// browser that goes away in the middle of a response.
type goneAfterFirstWrite struct {
	taken  bytes.Buffer
	writes int
}

func (writer *goneAfterFirstWrite) Write(contents []byte) (int, error) {
	if writer.writes > 0 {
		return 0, errBrowserGone
	}
	writer.writes++
	return writer.taken.Write(contents)
}

// preparedDownloadJob starts a download job for one small file and prepares
// its bytes, as the download handler finds them before it responds. The
// revision is the one the handler hands the browser as the ETag.
func preparedDownloadJob(t *testing.T, contents string) (*sftp.TransferManager, sftp.TransferJob, *sftp.PreparedDownload, string) {
	t.Helper()
	remote := remoteWith(map[string]node{"/file.bin": file("file.bin", contents, 0o644)})
	manager := newTestTransferManager(t, &sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }})
	job, err := manager.CreateJob(sftp.CreateTransferJob{
		ID: "transfer_response", BatchID: "batch_response", Alias: "edge", Direction: sftp.TransferDownload,
		Kind: sftp.TransferFile, Name: "file.bin", RemotePath: "/file.bin", TotalBytes: int64(len(contents)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob(job.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	prepared, err := manager.PrepareOwnedDownload(t.Context(), job.ID, job.Alias, job.RemotePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = prepared.Close() })
	return manager, job, prepared, strconv.Quote(prepared.Revision)
}

func TestADownloadResponseSentInFullIsVerifiedWithoutSendingItAgain(t *testing.T) {
	const contents = "payload!"
	manager, job, prepared, revision := preparedDownloadJob(t, contents)
	if err := manager.VerifyOwnedDownload(job.ID, prepared, revision); !errors.Is(err, sftp.ErrOffsetMismatch) {
		t.Fatalf("verify before any response = %v, want %v", err, sftp.ErrOffsetMismatch)
	}

	download, err := manager.BeginOwnedDownload(sftp.OwnedDownloadRequest{JobID: job.ID, Prepared: prepared, Revision: revision})
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	written, err := download.Send(t.Context(), &body)
	if err != nil || written != int64(len(contents)) || body.String() != contents {
		t.Fatalf("Send() = %d, %v with body %q, want %q", written, err, body.String(), contents)
	}
	if err := manager.VerifyOwnedDownload(job.ID, prepared, revision); err != nil {
		t.Fatalf("verify after the whole response = %v", err)
	}
}

func TestADownloadResponseCutShortIsVerifiedOnlyAfterTheRestIsSent(t *testing.T) {
	// Large enough that the response takes more than one write.
	contents := strings.Repeat("0123456789abcdef", (4<<20)/16)
	manager, job, prepared, revision := preparedDownloadJob(t, contents)
	first, err := manager.BeginOwnedDownload(sftp.OwnedDownloadRequest{JobID: job.ID, Prepared: prepared, Revision: revision})
	if err != nil {
		t.Fatal(err)
	}
	gone := &goneAfterFirstWrite{}
	written, err := first.Send(t.Context(), gone)
	if !errors.Is(err, errBrowserGone) || written == 0 || written >= int64(len(contents)) {
		t.Fatalf("Send() to a browser that went away = %d, %v", written, err)
	}
	if err := manager.VerifyOwnedDownload(job.ID, prepared, revision); !errors.Is(err, sftp.ErrOffsetMismatch) {
		t.Fatalf("verify after a cut response = %v, want %v", err, sftp.ErrOffsetMismatch)
	}

	// The browser keeps what it received and asks for the rest from there.
	if _, err := manager.AcknowledgeDownload(job.ID, written, revision); err != nil {
		t.Fatal(err)
	}
	rest, err := manager.BeginOwnedDownload(sftp.OwnedDownloadRequest{JobID: job.ID, Prepared: prepared, Revision: revision, Offset: written})
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	if _, err := rest.Send(t.Context(), &body); err != nil || gone.taken.String()+body.String() != contents {
		t.Fatalf("resumed Send() = %v, %d bytes after %d, want %d in all", err, body.Len(), gone.taken.Len(), len(contents))
	}
	if err := manager.VerifyOwnedDownload(job.ID, prepared, revision); err != nil {
		t.Fatalf("verify after the rest was sent = %v", err)
	}
}
