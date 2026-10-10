package sftp_test

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"sshc/internal/sftp"
)

// Each bounded fake transfer lasts a fraction of a second; allowing one burst
// window keeps scheduling jitter from making the lower-bound assertion brittle.
const speedPathTestRate = 1024
const speedPathBurstAllowance = 50 * time.Millisecond

func timedSpeedPath(t *testing.T, bytes int64, transfer func() error) {
	t.Helper()
	started := time.Now()
	if err := transfer(); err != nil {
		t.Fatal(err)
	}
	minimum := time.Duration(bytes)*time.Second/speedPathTestRate - speedPathBurstAllowance
	if elapsed := time.Since(started); elapsed < minimum {
		t.Fatalf("transfer bypassed the limit: %s, want >= %s", elapsed, minimum)
	}
}

func TestBrowserDownloadPreparationDeliveryAndZIPUseTheEngineSpeedLimit(t *testing.T) {
	content := strings.Repeat("x", 256)
	remote := remoteWith(map[string]node{"/folder": directory("folder"), "/folder/file": file("file", content, 0o600)})
	service := &sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return fakeConnection{remote}, nil }}
	manager := newTestTransferManager(t, service)
	settings := sftp.DefaultTransferSettings()
	settings.SpeedLimitBytesPerSecond = speedPathTestRate
	if err := manager.SetTransferSettings(settings); err != nil {
		t.Fatal(err)
	}
	var prepared *sftp.PreparedDownload
	timedSpeedPath(t, int64(len(content)), func() error {
		var err error
		prepared, err = service.PrepareDownloadForTest(t.Context(), "edge", "/folder/file")
		return err
	})
	defer prepared.Close()
	timedSpeedPath(t, int64(len(content)), func() error { _, err := prepared.WriteFrom(t.Context(), 0, io.Discard); return err })
	timedSpeedPath(t, int64(len(content)), func() error {
		_, err := service.DownloadArchive(t.Context(), "edge", "/folder", io.Discard)
		return err
	})
}

func TestBrowserUploadChunksUseTheEngineSpeedLimit(t *testing.T) {
	content := strings.Repeat("x", 256)
	remote := remoteWith(nil)
	manager := newTestTransferManager(t, &sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return fakeConnection{remote}, nil }})
	settings := sftp.DefaultTransferSettings()
	settings.SpeedLimitBytesPerSecond = speedPathTestRate
	if err := manager.SetTransferSettings(settings); err != nil {
		t.Fatal(err)
	}
	job, err := manager.CreateJob(sftp.CreateTransferJob{ID: "limited_upload01", BatchID: "limited_batch01", Alias: "edge", RemotePath: "/file", Direction: sftp.TransferUpload,
		Kind: sftp.TransferFile, Name: "file", TotalBytes: int64(len(content))})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob(job.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.StartOwned(t.Context(), job.Alias, job.ID, job.RemotePath, sftp.StartUploadOptions{Size: job.TotalBytes}); err != nil {
		t.Fatal(err)
	}
	timedSpeedPath(t, job.TotalBytes, func() error {
		_, err := manager.AppendOwned(t.Context(), sftp.UploadAppend{Target: sftp.UploadTarget{Alias: job.Alias, ID: job.ID, RemotePath: job.RemotePath}, Total: job.TotalBytes, Contents: []byte(content)})
		return err
	})
}
