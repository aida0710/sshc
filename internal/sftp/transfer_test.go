package sftp_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"sshc/internal/sftp"
)

func TestDownloadFromReturnsOnlyRemainingBytes(t *testing.T) {
	remote := remoteWith(map[string]node{"/large.bin": file("large.bin", "abcdefgh", 0o600)})
	service := serviceFor(remote)
	var output bytes.Buffer
	prepared, err := service.PrepareDownload(t.Context(), "edge", "/large.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	written, err := prepared.WriteFrom(t.Context(), 3, &output)
	if err != nil {
		t.Fatal(err)
	}
	if output.String() != "defgh" || written != 5 {
		t.Fatalf("download = %q / %d", output.String(), written)
	}
	if _, err := prepared.WriteFrom(t.Context(), 9, &output); !errors.Is(err, sftp.ErrOffsetMismatch) {
		t.Fatalf("past-end offset = %v", err)
	}
}

func TestPreparedDownloadSpoolIsReusedForTheOwningJob(t *testing.T) {
	remote := remoteWith(map[string]node{"/large.bin": file("large.bin", "abcdefgh", 0o600)})
	opens := 0
	service := &sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) {
		opens++
		return remote, nil
	}}
	manager := sftp.NewTransferManager(service)
	input := sftp.CreateTransferJob{
		ID: "transfer_spool01", BatchID: "batch_spool001", Alias: "edge", Direction: sftp.TransferDownload,
		Kind: sftp.TransferFile, Name: "large.bin", RemotePath: "/large.bin", TotalBytes: 8,
	}
	if _, err := manager.CreateJob(input); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob(input.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	first, err := manager.PrepareOwnedDownload(t.Context(), input.ID, input.Alias, input.RemotePath)
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.PrepareOwnedDownload(t.Context(), input.ID, input.Alias, input.RemotePath)
	if err != nil {
		t.Fatal(err)
	}
	_ = first.Close()
	_ = second.Close()
	if opens != 1 {
		t.Fatalf("remote spool opens = %d, want 1", opens)
	}
	const revision = `"content-sha256:cache"`
	if _, err := manager.BeginDownload(input.ID, 8, revision, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.RecordDownloadSent(input.ID, 8, 8, revision); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.AcknowledgeDownload(input.ID, 8, revision); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJobFromClient(input.ID, sftp.UpdateTransferJob{Action: sftp.TransferCompleteAction}); err != nil {
		t.Fatal(err)
	}
}

func TestPreparedDownloadSpoolBuildIsSingleflightPerJob(t *testing.T) {
	remote := remoteWith(map[string]node{"/large.bin": file("large.bin", "abcdefgh", 0o600)})
	opened := make(chan struct{})
	release := make(chan struct{})
	var hookOnce sync.Once
	remote.openHook = func(candidate string) {
		if candidate == "/large.bin" {
			hookOnce.Do(func() { close(opened) })
			<-release
		}
	}
	var opens atomic.Int32
	manager := sftp.NewTransferManager(&sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) {
		opens.Add(1)
		return remote, nil
	}})
	input := sftp.CreateTransferJob{ID: "transfer_singleflt", BatchID: "batch_singleflt", Alias: "edge", Direction: sftp.TransferDownload, Kind: sftp.TransferFile, Name: "large.bin", RemotePath: "/large.bin", TotalBytes: 8}
	if _, err := manager.CreateJob(input); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob(input.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	results := make(chan *sftp.PreparedDownload, 2)
	errorsResult := make(chan error, 2)
	for range 2 {
		go func() {
			prepared, err := manager.PrepareOwnedDownload(t.Context(), input.ID, input.Alias, input.RemotePath)
			results <- prepared
			errorsResult <- err
		}()
	}
	<-opened
	close(release)
	for range 2 {
		prepared := <-results
		if err := <-errorsResult; err != nil {
			t.Fatal(err)
		}
		if err := prepared.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if got := opens.Load(); got != 1 {
		t.Fatalf("remote opens = %d, want 1", got)
	}
}

func transferFingerprint(t *testing.T, contents []byte) string {
	t.Helper()
	result, err := sftp.SourceFingerprint(t.Context(), bytes.NewReader(contents), int64(len(contents)))
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestResumableUploadAppendsFromRemoteOffsetAndCompletesAtomically(t *testing.T) {
	remote := remoteWith(map[string]node{"/remote": directory("remote")})
	manager := sftp.NewTransferManager(&sftp.Service{
		Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil },
	})

	started, err := manager.Start(t.Context(), "edge", "transfer_12345678", "/remote/large.bin", sftp.StartUploadOptions{Size: 9})
	if err != nil || started.Offset != 0 || started.ExpectedRevision != sftp.AbsentRevision {
		t.Fatalf("Start() = %+v, %v", started, err)
	}
	if _, exists := remote.nodes["/remote/large.bin"]; exists {
		t.Fatal("target became visible before completion")
	}
	if _, err := manager.Append(t.Context(), "edge", started.ID, started.Path, 0, 9, []byte("large")); err != nil {
		t.Fatalf("Append(first) = %v", err)
	}
	resumed, err := manager.Start(t.Context(), "edge", started.ID, started.Path, sftp.StartUploadOptions{Size: 9, ExpectedRevision: sftp.AbsentRevision})
	if err != nil || resumed.Offset != 5 {
		t.Fatalf("Start(resume) = %+v, %v", resumed, err)
	}
	if _, err := manager.Append(t.Context(), "edge", started.ID, started.Path, 5, 9, []byte("file")); err != nil {
		t.Fatalf("Append(second) = %v", err)
	}
	completed, err := manager.Complete(t.Context(), "edge", started.ID, started.Path, 9, sftp.AbsentRevision, transferFingerprint(t, []byte("largefile")))
	if err != nil || completed.Bytes != 9 {
		t.Fatalf("Complete() = %+v, %v", completed, err)
	}
	if got := string(remote.nodes["/remote/large.bin"].content); got != "largefile" {
		t.Fatalf("target contents = %q", got)
	}
}

func TestResumableUploadReusesOneSFTPConnectionAcrossChunks(t *testing.T) {
	remote := remoteWith(map[string]node{"/remote": directory("remote")})
	opens := 0
	manager := sftp.NewTransferManager(&sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) {
		opens++
		return remote, nil
	}})
	started, err := manager.Start(t.Context(), "edge", "transfer_reuse1", "/remote/file.bin", sftp.StartUploadOptions{Size: 6})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Append(t.Context(), "edge", started.ID, started.Path, 0, 6, []byte("abc")); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Append(t.Context(), "edge", started.ID, started.Path, 3, 6, []byte("def")); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Complete(t.Context(), "edge", started.ID, started.Path, 6, started.ExpectedRevision, transferFingerprint(t, []byte("abcdef"))); err != nil {
		t.Fatal(err)
	}
	if opens != 1 {
		t.Fatalf("SFTP connections = %d, want 1", opens)
	}
}

func TestOwnedUploadCommitsProgressAndCompletionWithoutClientStateRequests(t *testing.T) {
	remote := remoteWith(map[string]node{"/remote": directory("remote")})
	manager := sftp.NewTransferManager(&sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) {
		return remote, nil
	}})
	input := sftp.CreateTransferJob{
		ID: "transfer_owned01", BatchID: "batch_owned001", Alias: "edge", Direction: sftp.TransferUpload,
		Kind: sftp.TransferFile, Name: "owned.bin", RemotePath: "/remote/owned.bin", TotalBytes: 6,
	}
	if _, err := manager.CreateJob(input); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob(input.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	started, err := manager.StartOwned(t.Context(), input.Alias, input.ID, input.RemotePath, sftp.StartUploadOptions{Size: input.TotalBytes})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.AppendOwned(t.Context(), input.Alias, input.ID, input.RemotePath, 0, input.TotalBytes, []byte("abcdef")); err != nil {
		t.Fatal(err)
	}
	jobs := manager.ListJobs()
	if len(jobs) != 1 || jobs[0].TransferredBytes != 6 || jobs[0].Status != sftp.TransferRunning {
		t.Fatalf("job after append = %+v", jobs)
	}
	if _, err := manager.CompleteOwned(t.Context(), input.Alias, input.ID, input.RemotePath, input.TotalBytes, started.ExpectedRevision, transferFingerprint(t, []byte("abcdef"))); err != nil {
		t.Fatal(err)
	}
	jobs = manager.ListJobs()
	if len(jobs) != 1 || jobs[0].TransferredBytes != 6 || jobs[0].Status != sftp.TransferCompleted {
		t.Fatalf("job after publish = %+v", jobs)
	}
	if got := string(remote.nodes[input.RemotePath].content); got != "abcdef" {
		t.Fatalf("published contents = %q", got)
	}
}

func TestStartOwnedResumesFromAnExistingRemotePart(t *testing.T) {
	remote := remoteWith(map[string]node{"/remote": directory("remote")})
	manager := sftp.NewTransferManager(&sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }})
	input := sftp.CreateTransferJob{ID: "transfer_resumeown", BatchID: "batch_resumeown", Alias: "edge", Direction: sftp.TransferUpload, Kind: sftp.TransferFile, Name: "file", RemotePath: "/remote/file", TotalBytes: 6}
	if _, err := manager.CreateJob(input); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob(input.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	started, err := manager.Start(t.Context(), input.Alias, input.ID, input.RemotePath, sftp.StartUploadOptions{Size: 6})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Append(t.Context(), input.Alias, input.ID, input.RemotePath, 0, 6, []byte("abc")); err != nil {
		t.Fatal(err)
	}
	resumed, err := manager.StartOwned(t.Context(), input.Alias, input.ID, input.RemotePath, sftp.StartUploadOptions{Size: 6, ExpectedRevision: started.ExpectedRevision})
	if err != nil || resumed.Offset != 3 {
		t.Fatalf("resume = %+v, %v", resumed, err)
	}
	if job := manager.ListJobs()[0]; job.TransferredBytes != 3 {
		t.Fatalf("job progress = %d", job.TransferredBytes)
	}
}

func TestAppendOwnedReplaysAnAcknowledgedChunkAfterResponseLoss(t *testing.T) {
	remote := remoteWith(map[string]node{"/remote": directory("remote")})
	manager := sftp.NewTransferManager(&sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }})
	input := sftp.CreateTransferJob{ID: "transfer_appendack", BatchID: "batch_appendack", Alias: "edge", Direction: sftp.TransferUpload, Kind: sftp.TransferFile, Name: "file", RemotePath: "/remote/file", TotalBytes: 4}
	if _, err := manager.CreateJob(input); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob(input.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.StartOwned(t.Context(), input.Alias, input.ID, input.RemotePath, sftp.StartUploadOptions{Size: 4}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.AppendOwned(t.Context(), input.Alias, input.ID, input.RemotePath, 0, 4, []byte("data")); err != nil {
		t.Fatal(err)
	}
	replayed, err := manager.AppendOwned(t.Context(), input.Alias, input.ID, input.RemotePath, 0, 4, []byte("data"))
	if err != nil || replayed.Offset != 4 {
		t.Fatalf("replayed append = %+v, %v", replayed, err)
	}
	part := "/remote/.file.sshc-upload-transfer_appendack.part"
	if got := string(remote.nodes[part].content); got != "data" {
		t.Fatalf("part after replay = %q", got)
	}
}

func TestCompleteOwnedIsIdempotentAfterResponseLoss(t *testing.T) {
	remote := remoteWith(map[string]node{"/remote": directory("remote")})
	manager := sftp.NewTransferManager(&sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }})
	input := sftp.CreateTransferJob{ID: "transfer_completeack", BatchID: "batch_completeack", Alias: "edge", Direction: sftp.TransferUpload, Kind: sftp.TransferFile, Name: "file", RemotePath: "/remote/file", TotalBytes: 4}
	if _, err := manager.CreateJob(input); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob(input.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	started, err := manager.StartOwned(t.Context(), input.Alias, input.ID, input.RemotePath, sftp.StartUploadOptions{Size: 4})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.AppendOwned(t.Context(), input.Alias, input.ID, input.RemotePath, 0, 4, []byte("data")); err != nil {
		t.Fatal(err)
	}
	fingerprint := transferFingerprint(t, []byte("data"))
	if _, err := manager.CompleteOwned(t.Context(), input.Alias, input.ID, input.RemotePath, 4, started.ExpectedRevision, fingerprint); err != nil {
		t.Fatal(err)
	}
	replayed, err := manager.CompleteOwned(t.Context(), input.Alias, input.ID, input.RemotePath, 4, started.ExpectedRevision, fingerprint)
	if err != nil || replayed.Path != input.RemotePath || replayed.Bytes != 4 {
		t.Fatalf("replayed complete = %+v, %v", replayed, err)
	}
	if len(remote.replacements) != 1 {
		t.Fatalf("publication count = %d", len(remote.replacements))
	}
}

func TestDownloadPrepareCannotInstallAfterCancellation(t *testing.T) {
	remote := remoteWith(map[string]node{"/large.bin": file("large.bin", "abcdefgh", 0o600)})
	opened := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	remote.openHook = func(candidate string) {
		if candidate == "/large.bin" {
			once.Do(func() { close(opened) })
			<-release
		}
	}
	manager := sftp.NewTransferManager(&sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }})
	input := sftp.CreateTransferJob{ID: "transfer_cancelprep", BatchID: "batch_cancelprep", Alias: "edge", Direction: sftp.TransferDownload, Kind: sftp.TransferFile, Name: "large.bin", RemotePath: "/large.bin", TotalBytes: 8}
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
	defer done()
	preparedResult := make(chan error, 1)
	go func() {
		prepared, prepareErr := manager.PrepareOwnedDownload(t.Context(), input.ID, input.Alias, input.RemotePath)
		if prepared != nil {
			_ = prepared.Close()
		}
		preparedResult <- prepareErr
	}()
	<-opened
	if _, err := manager.UpdateJobFromClient(input.ID, sftp.UpdateTransferJob{Action: sftp.TransferCancelAction}); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-preparedResult; !errors.Is(err, sftp.ErrTransferState) {
		t.Fatalf("prepare after cancel = %v", err)
	}
}

func TestPrepareOwnedArchiveFailsBeforeReturningAPartialDownload(t *testing.T) {
	remote := remoteWith(map[string]node{
		"/remote":          directory("remote"),
		"/remote/file.bin": file("file.bin", "payload", 0o600),
	})
	ctx, cancel := context.WithCancel(t.Context())
	remote.openHook = func(candidate string) {
		if candidate == "/remote/file.bin" {
			cancel()
		}
	}
	manager := sftp.NewTransferManager(&sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }})
	input := sftp.CreateTransferJob{ID: "transfer_archiveerr", BatchID: "batch_archiveerr", Alias: "edge", Direction: sftp.TransferDownload, Kind: sftp.TransferFolder, Name: "remote", RemotePath: "/remote", TotalBytes: -1}
	if _, err := manager.CreateJob(input); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob(input.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	prepared, err := manager.PrepareOwnedArchive(ctx, input.ID, input.Alias, input.RemotePath)
	if prepared != nil {
		_ = prepared.Close()
		t.Fatal("partial archive was returned")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("prepare archive = %v", err)
	}
	if job := manager.ListJobs()[0]; job.TransferredBytes != 0 || job.Status != sftp.TransferRunning {
		t.Fatalf("job after failed preparation = %+v", job)
	}
}

func TestRemoteCloseDoesNotHoldTheJobsMutex(t *testing.T) {
	remote := remoteWith(map[string]node{"/remote": directory("remote")})
	manager := sftp.NewTransferManager(&sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }})
	input := sftp.CreateTransferJob{ID: "transfer_closeout", BatchID: "batch_closeout", Alias: "edge", Direction: sftp.TransferUpload, Kind: sftp.TransferFile, Name: "file", RemotePath: "/remote/file", TotalBytes: 1}
	if _, err := manager.CreateJob(input); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob(input.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.StartOwned(t.Context(), input.Alias, input.ID, input.RemotePath, sftp.StartUploadOptions{Size: 1}); err != nil {
		t.Fatal(err)
	}
	closeStarted := make(chan struct{})
	releaseClose := make(chan struct{})
	var once sync.Once
	remote.closeHook = func() {
		once.Do(func() { close(closeStarted) })
		<-releaseClose
	}
	pauseResult := make(chan error, 1)
	go func() {
		_, pauseErr := manager.UpdateJobFromClient(input.ID, sftp.UpdateTransferJob{Action: sftp.TransferPauseAction})
		pauseResult <- pauseErr
	}()
	<-closeStarted
	listed := make(chan struct{})
	go func() {
		_ = manager.ListJobs()
		close(listed)
	}()
	select {
	case <-listed:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("ListJobs blocked on Remote.Close while jobsMutex was held")
	}
	close(releaseClose)
	if err := <-pauseResult; err != nil {
		t.Fatal(err)
	}
}

func TestCancelOwnedRetainsCleanupFailureForRetry(t *testing.T) {
	remote := remoteWith(map[string]node{"/remote": directory("remote")})
	manager := sftp.NewTransferManager(&sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }})
	input := sftp.CreateTransferJob{ID: "transfer_cleanretry", BatchID: "batch_cleanretry", Alias: "edge", Direction: sftp.TransferUpload, Kind: sftp.TransferFile, Name: "file", RemotePath: "/remote/file", TotalBytes: 4}
	if _, err := manager.CreateJob(input); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob(input.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.StartOwned(t.Context(), input.Alias, input.ID, input.RemotePath, sftp.StartUploadOptions{Size: 4}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.AppendOwned(t.Context(), input.Alias, input.ID, input.RemotePath, 0, 4, []byte("data")); err != nil {
		t.Fatal(err)
	}
	remote.removeErr = errors.New("temporary cleanup failure")
	if err := manager.CancelOwned(t.Context(), input.Alias, input.ID, input.RemotePath); err == nil {
		t.Fatal("cleanup failure was hidden")
	}
	if job := manager.ListJobs()[0]; job.Status != sftp.TransferFailed || job.Problem != "sftp_cleanup_pending" {
		t.Fatalf("job after cleanup failure = %+v", job)
	}
	remote.removeErr = nil
	if err := manager.CancelOwned(t.Context(), input.Alias, input.ID, input.RemotePath); err != nil {
		t.Fatal(err)
	}
	if job := manager.ListJobs()[0]; job.Status != sftp.TransferCancelled {
		t.Fatalf("job after cleanup retry = %+v", job)
	}
	part := "/remote/.file.sshc-upload-transfer_cleanretry.part"
	if _, err := remote.Lstat(part); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("part remains after cleanup retry: %v", err)
	}
}

func TestCancelOwnedRemovesOnlyTheDeterministicPartAfterJobStateLoss(t *testing.T) {
	const (
		id     = "transfer_lostjob1"
		target = "/remote/file"
		part   = "/remote/.file.sshc-upload-transfer_lostjob1.part"
	)
	remote := remoteWith(map[string]node{
		"/remote": directory("remote"),
		target:    file("file", "published", 0o600),
		part:      file(".file.sshc-upload-transfer_lostjob1.part", "partial", 0o600),
	})
	manager := sftp.NewTransferManager(&sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }})
	if err := manager.CancelOwned(t.Context(), "edge", id, target); err != nil {
		t.Fatalf("idempotent cancellation after job loss = %v", err)
	}
	if _, err := remote.Lstat(part); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("lost-job part remains: %v", err)
	}
	if info, err := remote.Lstat(target); err != nil || string(info.(node).content) != "published" {
		t.Fatalf("published target changed: %v, %v", info, err)
	}
	removals := len(remote.removals)
	if err := manager.CancelOwned(t.Context(), "edge", "bad", target); !errors.Is(err, sftp.ErrInvalidTransfer) {
		t.Fatalf("invalid id cancellation = %v", err)
	}
	if err := manager.CancelOwned(t.Context(), "edge", id, "../../file"); err == nil {
		t.Fatal("traversal cancellation succeeded")
	}
	if len(remote.removals) != removals {
		t.Fatal("invalid cancellation reached remote removal")
	}
}

func TestCompleteOwnedCommitsAfterReplaceEvenWhenTargetLstatFails(t *testing.T) {
	remote := remoteWith(map[string]node{"/remote": directory("remote")})
	manager := sftp.NewTransferManager(&sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }})
	input := sftp.CreateTransferJob{ID: "transfer_commitpt", BatchID: "batch_commitpt1", Alias: "edge", Direction: sftp.TransferUpload, Kind: sftp.TransferFile, Name: "file", RemotePath: "/remote/file", TotalBytes: 4}
	if _, err := manager.CreateJob(input); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob(input.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	started, err := manager.StartOwned(t.Context(), input.Alias, input.ID, input.RemotePath, sftp.StartUploadOptions{Size: 4})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.AppendOwned(t.Context(), input.Alias, input.ID, input.RemotePath, 0, 4, []byte("data")); err != nil {
		t.Fatal(err)
	}
	remote.lstatHook = func(candidate string) error {
		if candidate == input.RemotePath && len(remote.replacements) > 0 {
			return errors.New("post-publish stat failed")
		}
		return nil
	}
	if _, err := manager.CompleteOwned(t.Context(), input.Alias, input.ID, input.RemotePath, 4, started.ExpectedRevision, transferFingerprint(t, []byte("data"))); err != nil {
		t.Fatal(err)
	}
	if job := manager.ListJobs()[0]; job.Status != sftp.TransferCompleted {
		t.Fatalf("job = %+v", job)
	}
	if got := string(remote.nodes[input.RemotePath].content); got != "data" {
		t.Fatalf("published = %q", got)
	}
}

func TestUploadPublicationIsSerializedWithClientControls(t *testing.T) {
	remote := remoteWith(map[string]node{"/remote": directory("remote")})
	replaceStarted := make(chan struct{})
	releaseReplace := make(chan struct{})
	var once sync.Once
	remote.replaceHook = func() {
		once.Do(func() { close(replaceStarted) })
		<-releaseReplace
	}
	manager := sftp.NewTransferManager(&sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }})
	input := sftp.CreateTransferJob{
		ID: "transfer_publish", BatchID: "batch_publish1", Alias: "edge", Direction: sftp.TransferUpload,
		Kind: sftp.TransferFile, Name: "file", RemotePath: "/remote/file", TotalBytes: 4,
	}
	if _, err := manager.CreateJob(input); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJobFromClient(input.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	started, err := manager.StartOwned(t.Context(), input.Alias, input.ID, input.RemotePath, sftp.StartUploadOptions{Size: 4})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.AppendOwned(t.Context(), input.Alias, input.ID, input.RemotePath, 0, 4, []byte("data")); err != nil {
		t.Fatal(err)
	}
	fingerprint := transferFingerprint(t, []byte("data"))
	completeResult := make(chan error, 1)
	go func() {
		_, completeErr := manager.CompleteOwned(t.Context(), input.Alias, input.ID, input.RemotePath, 4, started.ExpectedRevision, fingerprint)
		completeResult <- completeErr
	}()
	<-replaceStarted
	pauseResult := make(chan error, 1)
	go func() {
		_, pauseErr := manager.UpdateJobFromClient(input.ID, sftp.UpdateTransferJob{Action: sftp.TransferPauseAction})
		pauseResult <- pauseErr
	}()
	select {
	case err := <-pauseResult:
		t.Fatalf("pause raced publication: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(releaseReplace)
	if err := <-completeResult; err != nil {
		t.Fatal(err)
	}
	if err := <-pauseResult; !errors.Is(err, sftp.ErrTransferState) {
		t.Fatalf("pause after publish = %v", err)
	}
	if got := manager.ListJobs()[0].Status; got != sftp.TransferCompleted {
		t.Fatalf("status = %s", got)
	}
}

func TestResumableUploadRejectsOffsetAndTargetConflicts(t *testing.T) {
	remote := remoteWith(map[string]node{
		"/remote":              directory("remote"),
		"/remote/existing.txt": file("existing.txt", "old", 0o640),
	})
	manager := sftp.NewTransferManager(&sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }})
	started, err := manager.Start(t.Context(), "edge", "transfer_abcdefgh", "/remote/existing.txt", sftp.StartUploadOptions{Size: 3, Overwrite: true})
	if err != nil {
		t.Fatalf("Start() = %v", err)
	}
	if _, err := manager.Append(t.Context(), "edge", started.ID, started.Path, 1, 3, []byte("new")); !errors.Is(err, sftp.ErrOffsetMismatch) {
		t.Fatalf("wrong offset = %v", err)
	}
	if _, err := manager.Append(t.Context(), "edge", started.ID, started.Path, 0, 3, []byte("new")); err != nil {
		t.Fatalf("Append() = %v", err)
	}
	changed := remote.nodes["/remote/existing.txt"]
	// Keep size, mode and SFTP v3's second-resolution mtime identical. Content
	// revision binding must still reject this replacement.
	changed.content = []byte("bad")
	remote.nodes["/remote/existing.txt"] = changed
	if _, err := manager.Complete(t.Context(), "edge", started.ID, started.Path, 3, started.ExpectedRevision, transferFingerprint(t, []byte("new"))); !errors.Is(err, sftp.ErrConflict) {
		t.Fatalf("Complete(stale) = %v", err)
	}
}

func TestCompleteRejectsAPartWhoseFingerprintDiffersFromTheSelectedSource(t *testing.T) {
	remote := remoteWith(map[string]node{"/remote": directory("remote")})
	manager := sftp.NewTransferManager(&sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }})
	started, err := manager.Start(t.Context(), "edge", "transfer_digest1", "/remote/digest.bin", sftp.StartUploadOptions{Size: 4})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Append(t.Context(), "edge", started.ID, started.Path, 0, 4, []byte("evil")); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Complete(t.Context(), "edge", started.ID, started.Path, 4, started.ExpectedRevision, transferFingerprint(t, []byte("good"))); !errors.Is(err, sftp.ErrConflict) {
		t.Fatalf("mismatched fingerprint = %v", err)
	}
	if _, err := remote.Lstat(started.Path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("mismatched part was published: %v", err)
	}
}

func TestResumableUploadCancelRemovesOnlyPartFile(t *testing.T) {
	remote := remoteWith(map[string]node{"/remote": directory("remote")})
	manager := sftp.NewTransferManager(&sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }})
	started, err := manager.Start(t.Context(), "edge", "transfer_cancel1", "/remote/cancel.bin", sftp.StartUploadOptions{Size: 8})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Append(t.Context(), "edge", started.ID, started.Path, 0, 8, []byte("partial")); err != nil {
		t.Fatal(err)
	}
	if err := manager.Cancel(t.Context(), "edge", started.ID, started.Path); err != nil {
		t.Fatal(err)
	}
	for candidate := range remote.nodes {
		if strings.Contains(candidate, started.ID) {
			t.Fatalf("part remains at %q", candidate)
		}
	}
	if _, err := remote.Lstat("/remote/cancel.bin"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("target exists after cancel: %v", err)
	}
}

func TestListHidesReservedResumablePartFiles(t *testing.T) {
	remote := remoteWith(map[string]node{
		"/remote":             directory("remote"),
		"/remote/visible.txt": file("visible.txt", "done", 0o600),
		"/remote/.large.bin.sshc-upload-transfer_12345678.part": file(".large.bin.sshc-upload-transfer_12345678.part", "partial", 0o600),
	})
	entries, err := serviceFor(remote).List(t.Context(), "edge", "/remote")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "visible.txt" {
		t.Fatalf("entries = %#v", entries)
	}
}

func TestTwoUploadsForTheSameAbsentTargetCannotBothComplete(t *testing.T) {
	remote := remoteWith(map[string]node{"/remote": directory("remote")})
	manager := sftp.NewTransferManager(&sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }})
	first, err := manager.Start(t.Context(), "edge", "transfer_first1", "/remote/same.bin", sftp.StartUploadOptions{Size: 3})
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.Start(t.Context(), "edge", "transfer_second", "/remote/same.bin", sftp.StartUploadOptions{Size: 3})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Append(t.Context(), "edge", first.ID, first.Path, 0, 3, []byte("one")); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Append(t.Context(), "edge", second.ID, second.Path, 0, 3, []byte("two")); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Complete(t.Context(), "edge", first.ID, first.Path, 3, first.ExpectedRevision, transferFingerprint(t, []byte("one"))); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Complete(t.Context(), "edge", second.ID, second.Path, 3, second.ExpectedRevision, transferFingerprint(t, []byte("two"))); !errors.Is(err, sftp.ErrAlreadyExists) {
		t.Fatalf("second Complete() = %v", err)
	}
}
