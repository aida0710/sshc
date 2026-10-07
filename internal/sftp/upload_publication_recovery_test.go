package sftp_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"sshc/internal/sftp"
)

func TestUploadPublicationWithoutAnAcknowledgementCannotBeReplayedAndCancelKeepsTheTarget(t *testing.T) {
	remote := publicationAcknowledgementLostRemote{remoteWith(nil)}
	manager := newTestTransferManager(t, &sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }})
	settings := sftp.DefaultTransferSettings()
	settings.AutoReconnect, settings.MaxReconnectAttempts = true, 3
	if err := manager.SetTransferSettings(settings); err != nil {
		t.Fatal(err)
	}
	job, err := manager.CreateJob(sftp.CreateTransferJob{ID: "publish_upload01", BatchID: "publish_batch01", Alias: "edge", RemotePath: "/target",
		Direction: sftp.TransferUpload, Kind: sftp.TransferFile, Name: "target", TotalBytes: 7})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob(job.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	fingerprint, err := sftp.SourceFingerprint(t.Context(), strings.NewReader("payload"), job.TotalBytes)
	if err != nil {
		t.Fatal(err)
	}
	target := sftp.UploadTarget{Alias: job.Alias, ID: job.ID, RemotePath: job.RemotePath}
	started, err := manager.StartOwned(t.Context(), job.Alias, job.ID, job.RemotePath, sftp.StartUploadOptions{Size: job.TotalBytes, SourceFingerprint: fingerprint})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.AppendOwned(t.Context(), sftp.UploadAppend{Target: target, Total: job.TotalBytes, Contents: []byte("payload")}); err != nil {
		t.Fatal(err)
	}
	_, err = manager.CompleteOwned(t.Context(), sftp.UploadCompletion{Target: target, Total: job.TotalBytes, ExpectedRevision: started.ExpectedRevision, SourceFingerprint: fingerprint})
	if !errors.Is(err, sftp.ErrAmbiguousTransfer) || sftp.ConnectionLost(err) {
		t.Fatalf("uncertain completion = %v", err)
	}
	failed := awaitRecoveryStatus(t, manager, sftp.TransferFailed)
	if failed.Problem != sftp.RemoteReconciliationProblem || len(sftp.AllowedTransferActions(failed)) != 1 || sftp.AllowedTransferActions(failed)[0] != sftp.TransferCancelControl {
		t.Fatalf("uncertain upload = %+v", failed)
	}
	if _, err := manager.UpdateJobFromClient(job.ID, sftp.UpdateTransferJob{Action: sftp.TransferRetryAction}); !errors.Is(err, sftp.ErrTransferState) {
		t.Fatalf("uncertain upload was allowed to repeat: %v", err)
	}
	if err := manager.CancelOwned(t.Context(), job.Alias, job.ID, job.RemotePath); err != nil {
		t.Fatal(err)
	}
	if string(remote.nodes["/target"].content) != "payload" {
		t.Fatal("cancel removed the published target")
	}
}
