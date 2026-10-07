package sftp_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"sshc/internal/sftp"
)

type interruptedFileRemote struct {
	*fakeRemote
	interrupt         atomic.Bool
	interruptCreation atomic.Bool
	creates           atomic.Int32
	writes            atomic.Int64
}

func (remote *interruptedFileRemote) OpenFile(name string, flags int) (sftp.WriteSeekCloser, error) {
	if remote.interruptCreation.CompareAndSwap(true, false) {
		return nil, io.EOF
	}
	file, err := remote.fakeRemote.OpenFile(name, flags)
	if err != nil {
		return nil, err
	}
	remote.creates.Add(1)
	return &interruptedFileWriter{WriteSeekCloser: file, remote: remote}, nil
}

type interruptedFileWriter struct {
	sftp.WriteSeekCloser
	remote *interruptedFileRemote
}

func (writer *interruptedFileWriter) Write(contents []byte) (int, error) {
	if writer.remote.interrupt.CompareAndSwap(true, false) {
		written, err := writer.WriteSeekCloser.Write(contents[:len(contents)/2])
		writer.remote.writes.Add(int64(written))
		if err != nil {
			return written, err
		}
		return written, io.EOF
	}
	written, err := writer.WriteSeekCloser.Write(contents)
	writer.remote.writes.Add(int64(written))
	return written, err
}

func queuedFileRecovery(t *testing.T, operation sftp.RemoteTransferOperation) (*sftp.TransferManager, *interruptedFileRemote, *fakeRemote, *atomic.Int32) {
	t.Helper()
	content := strings.Repeat("checked payload", 100)
	source := remoteWith(map[string]node{"/source": file("source", content, 0o640)})
	target := &interruptedFileRemote{fakeRemote: remoteWith(nil)}
	target.interrupt.Store(true)
	opens := &atomic.Int32{}
	service := &sftp.Service{Open: func(_ context.Context, alias string) (sftp.Remote, error) {
		if alias == "source" {
			opens.Add(1)
			return fakeConnection{source}, nil
		}
		return target, nil
	}}
	manager := newTestTransferManager(t, service)
	createFileRecoveryJob(t, manager, sftp.CreateTransferJob{
		Alias: "target", SourceAlias: "source", SourcePath: "/source", RemotePath: "/target", Operation: operation,
		TotalBytes: int64(len(content)),
	})
	return manager, target, source, opens
}

func createFileRecoveryJob(t *testing.T, manager *sftp.TransferManager, input sftp.CreateTransferJob) {
	t.Helper()
	settings := sftp.DefaultTransferSettings()
	settings.AutoReconnect, settings.MaxReconnectAttempts = true, 2
	if err := manager.SetTransferSettings(settings); err != nil {
		t.Fatal(err)
	}
	input.ID, input.BatchID = "recover_file_01", "recover_batch_01"
	input.BatchName, input.Name = "file", "source"
	input.BatchKind, input.Kind, input.Direction = sftp.TransferFile, sftp.TransferFile, sftp.TransferRemote
	if _, err := manager.CreateJob(input); err != nil {
		t.Fatal(err)
	}
}

func awaitRecoveryStatus(t *testing.T, manager *sftp.TransferManager, status sftp.TransferJobStatus) sftp.TransferJob {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticks := time.NewTicker(time.Millisecond)
	defer ticks.Stop()
	for {
		jobs, err := manager.ListJobs()
		if err != nil {
			t.Fatal(err)
		}
		if len(jobs) == 1 && jobs[0].Status == status {
			return jobs[0]
		}
		select {
		case <-deadline.C:
			t.Fatalf("wanted %s; jobs = %+v", status, jobs)
		case <-ticks.C:
		}
	}
}

func TestDisconnectedServerCopyReconnectsAndWritesOnlyTheVerifiedSuffix(t *testing.T) {
	manager, target, source, opens := queuedFileRecovery(t, sftp.RemoteCopy)
	job := awaitRecoveryStatus(t, manager, sftp.TransferCompleted)
	if job.ReconnectAttempt != 1 || opens.Load() < 4 {
		t.Fatalf("reconnect = %+v, opens = %d", job, opens.Load())
	}
	if !bytes.Equal(target.nodes["/target"].content, source.nodes["/source"].content) {
		t.Fatal("resumed contents differ")
	}
	if target.writes.Load() != int64(len(source.nodes["/source"].content)) {
		t.Fatalf("resumed prefix was sent again: %d", target.writes.Load())
	}
}

func TestLosingTheConnectionBeforePartCreationRecoversWithoutDroppingTheCheckpoint(t *testing.T) {
	content := "verified source"
	source := remoteWith(map[string]node{"/source": file("source", content, 0o640)})
	target := &interruptedFileRemote{fakeRemote: remoteWith(nil)}
	target.interruptCreation.Store(true)
	manager := newTestTransferManager(t, &sftp.Service{Open: func(_ context.Context, alias string) (sftp.Remote, error) {
		if alias == "source" {
			return fakeConnection{source}, nil
		}
		return target, nil
	}})
	createFileRecoveryJob(t, manager, sftp.CreateTransferJob{Alias: "target", SourceAlias: "source", SourcePath: "/source", RemotePath: "/target",
		Operation: sftp.RemoteCopy, TotalBytes: int64(len(content))})
	waiting := awaitRecoveryStatus(t, manager, sftp.TransferReconnecting)
	if waiting.RemoteCheckpoint == nil || waiting.TransferredBytes != 0 {
		t.Fatalf("creation checkpoint = %+v", waiting)
	}
	job := awaitRecoveryStatus(t, manager, sftp.TransferCompleted)
	if job.ReconnectAttempt != 1 || target.creates.Load() != 1 || string(target.nodes["/target"].content) != content {
		t.Fatalf("creation recovery = %+v, creations = %d", job, target.creates.Load())
	}
}

func TestPausingAReconnectWaitPreventsAnotherConnectionAttempt(t *testing.T) {
	manager, _, _, opens := queuedFileRecovery(t, sftp.RemoteCopy)
	awaitRecoveryStatus(t, manager, sftp.TransferReconnecting)
	opened := opens.Load()
	if _, err := manager.UpdateJobFromClient("recover_file_01", sftp.UpdateTransferJob{Action: sftp.TransferPauseAction}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	job := awaitRecoveryStatus(t, manager, sftp.TransferPaused)
	if opens.Load() != opened || len(sftp.AllowedTransferActions(job)) == 0 {
		t.Fatal("paused recovery kept reconnecting")
	}
}

func TestChangingSourceContentsWithTheSameMetadataStopsRecovery(t *testing.T) {
	manager, _, source, _ := queuedFileRecovery(t, sftp.RemoteCopy)
	awaitRecoveryStatus(t, manager, sftp.TransferReconnecting)
	changed := source.nodes["/source"]
	changed.content = bytes.Repeat([]byte("X"), len(changed.content))
	source.nodes["/source"] = changed
	job := awaitRecoveryStatus(t, manager, sftp.TransferFailed)
	if job.Problem != "sftp_conflict" || job.ReconnectAttempt != 1 {
		t.Fatalf("changed source = %+v", job)
	}
}

func TestChangingAnInterruptedPartStopsRecoveryInsteadOfJoiningDifferentContents(t *testing.T) {
	manager, target, _, _ := queuedFileRecovery(t, sftp.RemoteCopy)
	awaitRecoveryStatus(t, manager, sftp.TransferReconnecting)
	for name, partial := range target.nodes {
		if strings.HasSuffix(name, ".part") {
			partial.content[0] ^= 1
			target.nodes[name] = partial
		}
	}
	job := awaitRecoveryStatus(t, manager, sftp.TransferFailed)
	if job.Problem != "sftp_conflict" {
		t.Fatalf("changed part = %+v", job)
	}
}

func TestCancellingAndShuttingDownAReconnectWaitEndsItsWorker(t *testing.T) {
	for _, cancelJob := range []bool{true, false} {
		t.Run(map[bool]string{true: "cancel", false: "shutdown"}[cancelJob], func(t *testing.T) {
			manager, _, _, opens := queuedFileRecovery(t, sftp.RemoteCopy)
			awaitRecoveryStatus(t, manager, sftp.TransferReconnecting)
			opened := opens.Load()
			if cancelJob {
				if _, err := manager.UpdateJobFromClient("recover_file_01", sftp.UpdateTransferJob{Action: sftp.TransferCancelAction}); err != nil {
					t.Fatal(err)
				}
			}
			if err := manager.Close(); err != nil {
				t.Fatal(err)
			}
			if opens.Load() != opened {
				t.Fatal("stopped worker made another connection attempt")
			}
		})
	}
}

func TestTransportRecoveryStopsAtTheConfiguredAttemptBudget(t *testing.T) {
	var opens atomic.Int32
	manager := newTestTransferManager(t, &sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { opens.Add(1); return nil, io.EOF }})
	settings := sftp.DefaultTransferSettings()
	settings.AutoReconnect, settings.MaxReconnectAttempts = true, 1
	if err := manager.SetTransferSettings(settings); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.CreateJob(sftp.CreateTransferJob{ID: "bounded_recovery", BatchID: "bounded_batch", Alias: "target", SourceAlias: "source",
		SourcePath: "/source", RemotePath: "/target", Operation: sftp.RemoteCopy, Direction: sftp.TransferRemote, Kind: sftp.TransferFile, Name: "source", TotalBytes: 5}); err != nil {
		t.Fatal(err)
	}
	job := awaitRecoveryStatus(t, manager, sftp.TransferFailed)
	if job.ReconnectAttempt != 1 || opens.Load() != 2 {
		t.Fatalf("unbounded recovery = %+v; opens = %d", job, opens.Load())
	}
}

type interruptedReadRemote struct {
	*fakeRemote
	opens atomic.Int32
}

func (remote *interruptedReadRemote) Open(name string) (io.ReadCloser, error) {
	input, err := remote.fakeRemote.Open(name)
	if err != nil || remote.opens.Add(1) != 2 {
		return input, err
	}
	// The first read hashes the source; the second is the interrupted transfer.
	return &interruptedFileReader{ReadCloser: input}, nil
}

type interruptedFileReader struct {
	io.ReadCloser
}

func (reader *interruptedFileReader) Read(contents []byte) (int, error) {
	read, err := reader.ReadCloser.Read(contents[:max(1, len(contents)/2)])
	if err != nil {
		return read, err
	}
	return read, io.ErrUnexpectedEOF
}

func TestDisconnectedServerGetResumesItsVerifiedLocalPart(t *testing.T) {
	content := strings.Repeat("download", 100)
	source := &interruptedReadRemote{fakeRemote: remoteWith(map[string]node{"/source": file("source", content, 0o640)})}
	manager := newTestTransferManager(t, &sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return source, nil }})
	target := filepath.Join(t.TempDir(), "target")
	createFileRecoveryJob(t, manager, sftp.CreateTransferJob{Alias: "source", SourceAlias: "source", SourcePath: "/source", RemotePath: filepath.ToSlash(target),
		Operation: sftp.RemoteGet, TotalBytes: int64(len(content))})
	waiting := awaitRecoveryStatus(t, manager, sftp.TransferReconnecting)
	if waiting.TransferredBytes == 0 || waiting.TransferredBytes >= int64(len(content)) {
		t.Fatalf("local checkpoint = %+v", waiting)
	}
	job := awaitRecoveryStatus(t, manager, sftp.TransferCompleted)
	if job.ReconnectAttempt != 1 {
		t.Fatalf("reconnect = %+v", job)
	}
	contents, err := os.ReadFile(target)
	if err != nil || string(contents) != content {
		t.Fatalf("local target = %q, %v", contents, err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(target), ".sshc-download-"+job.ID)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("published part remained: %v", err)
	}
}

func TestDisconnectedServerPutWritesOnlyTheVerifiedSuffix(t *testing.T) {
	content := strings.Repeat("upload", 100)
	source := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(source, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	target := &interruptedFileRemote{fakeRemote: remoteWith(nil)}
	target.interrupt.Store(true)
	manager := newTestTransferManager(t, &sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return target, nil }})
	createFileRecoveryJob(t, manager, sftp.CreateTransferJob{Alias: "target", SourceAlias: "target", SourcePath: filepath.ToSlash(source), RemotePath: "/target",
		Operation: sftp.RemotePut, TotalBytes: int64(len(content))})
	job := awaitRecoveryStatus(t, manager, sftp.TransferCompleted)
	if job.ReconnectAttempt != 1 || target.writes.Load() != int64(len(content)) || string(target.nodes["/target"].content) != content {
		t.Fatalf("put = %+v, bytes sent = %d", job, target.writes.Load())
	}
}

type publicationAcknowledgementLostRemote struct{ *fakeRemote }

func (remote publicationAcknowledgementLostRemote) Rename(source, target string) error {
	if err := remote.fakeRemote.Rename(source, target); err != nil {
		return err
	}
	return io.EOF
}

func TestLosingPublicationAcknowledgementRequiresInspectionWithoutReconnect(t *testing.T) {
	source := remoteWith(map[string]node{"/source": file("source", "once", 0o640)})
	target := remoteWith(nil)
	manager := newTestTransferManager(t, &sftp.Service{Open: func(_ context.Context, alias string) (sftp.Remote, error) {
		if alias == "source" {
			return fakeConnection{source}, nil
		}
		return publicationAcknowledgementLostRemote{target}, nil
	}})
	createFileRecoveryJob(t, manager, sftp.CreateTransferJob{Alias: "target", SourceAlias: "source", SourcePath: "/source", RemotePath: "/target", Operation: sftp.RemoteCopy, TotalBytes: 4})
	job := awaitRecoveryStatus(t, manager, sftp.TransferFailed)
	if job.ReconnectAttempt != 0 || job.Problem != sftp.RemoteReconciliationProblem || string(target.nodes["/target"].content) != "once" {
		t.Fatalf("ambiguous publication = %+v", job)
	}
	if actions := sftp.AllowedTransferActions(job); len(actions) != 1 || actions[0] != sftp.TransferCancelControl {
		t.Fatalf("ambiguous publication actions = %v", actions)
	}
}

func TestChangingDestinationContentsWithTheSameMetadataStopsRecovery(t *testing.T) {
	source := remoteWith(map[string]node{"/source": file("source", "replacement", 0o640)})
	target := &interruptedFileRemote{fakeRemote: remoteWith(map[string]node{"/target": file("target", "original", 0o640)})}
	target.interrupt.Store(true)
	service := twoHostService(fakeConnection{source}, target)
	manager := newTestTransferManager(t, &service)
	createFileRecoveryJob(t, manager, sftp.CreateTransferJob{Alias: "target", SourceAlias: "source", SourcePath: "/source", RemotePath: "/target", Operation: sftp.RemoteCopy, TotalBytes: 11, Overwrite: true})
	awaitRecoveryStatus(t, manager, sftp.TransferReconnecting)
	changed := target.nodes["/target"]
	changed.content = []byte("modified")
	target.nodes["/target"] = changed
	job := awaitRecoveryStatus(t, manager, sftp.TransferFailed)
	if job.Problem != "sftp_conflict" || string(target.nodes["/target"].content) != "modified" {
		t.Fatalf("changed target = %+v", job)
	}
}

func TestStoppingTheQueueOrDisablingRecoveryEndsTheReconnectWait(t *testing.T) {
	for _, stopQueue := range []bool{true, false} {
		t.Run(map[bool]string{true: "stop queue", false: "disable recovery"}[stopQueue], func(t *testing.T) {
			manager, _, _, opens := queuedFileRecovery(t, sftp.RemoteCopy)
			awaitRecoveryStatus(t, manager, sftp.TransferReconnecting)
			opened := opens.Load()
			settings := sftp.DefaultTransferSettings()
			settings.ProcessingStopped, settings.AutoReconnect, settings.MaxReconnectAttempts = stopQueue, stopQueue, 2
			if err := manager.SetTransferSettings(settings); err != nil {
				t.Fatal(err)
			}
			awaitRecoveryStatus(t, manager, sftp.TransferFailed)
			if err := manager.Close(); err != nil {
				t.Fatal(err)
			}
			if opens.Load() != opened {
				t.Fatal("stopped recovery opened another source connection")
			}
		})
	}
}

func TestRefusedOrNonIdempotentServerJobsNeverReconnect(t *testing.T) {
	for _, request := range []struct {
		name      string
		operation sftp.RemoteTransferOperation
		refusal   error
	}{
		{name: "authentication", operation: sftp.RemoteCopy, refusal: errors.New("ssh: unable to authenticate")},
		{name: "permission", operation: sftp.RemoteCopy, refusal: fs.ErrPermission},
		{name: "revision", operation: sftp.RemoteCopy, refusal: sftp.ErrConflict},
		{name: "move", operation: sftp.RemoteMove, refusal: io.EOF},
		{name: "delete", operation: sftp.RemoteDelete, refusal: io.EOF},
	} {
		t.Run(request.name, func(t *testing.T) {
			var opens atomic.Int32
			manager := newTestTransferManager(t, &sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { opens.Add(1); return nil, request.refusal }})
			createFileRecoveryJob(t, manager, sftp.CreateTransferJob{Alias: "source", SourceAlias: "source", SourcePath: "/source", RemotePath: "/source", Operation: request.operation, TotalBytes: 4})
			job := awaitRecoveryStatus(t, manager, sftp.TransferFailed)
			if opens.Load() != 1 || job.ReconnectAttempt != 0 {
				t.Fatalf("refused job = %+v, connections = %d", job, opens.Load())
			}
		})
	}
}

func TestEngineRestartLeavesAReconnectWaitPausedWithItsCheckpoint(t *testing.T) {
	// Persistence is enabled before the job is admitted, as in engine startup.
	source := remoteWith(map[string]node{"/source": file("source", "persistent payload", 0o640)})
	target := &interruptedFileRemote{fakeRemote: remoteWith(nil)}
	target.interrupt.Store(true)
	service := twoHostService(fakeConnection{source}, target)
	manager := newTestTransferManager(t, &service)
	queue := filepath.Join(t.TempDir(), "queue.json")
	if err := manager.EnableQueuePersistence(queue); err != nil {
		t.Fatal(err)
	}
	createFileRecoveryJob(t, manager, sftp.CreateTransferJob{Alias: "target", SourceAlias: "source", SourcePath: "/source", RemotePath: "/target", Operation: sftp.RemoteCopy, TotalBytes: 18})
	waiting := awaitRecoveryStatus(t, manager, sftp.TransferReconnecting)
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	restored := newTestTransferManager(t, &service)
	if err := restored.EnableQueuePersistence(queue); err != nil {
		t.Fatal(err)
	}
	paused := awaitRecoveryStatus(t, restored, sftp.TransferPaused)
	if paused.RemoteCheckpoint == nil || paused.TransferredBytes != waiting.TransferredBytes || !paused.ReconnectAt.IsZero() {
		t.Fatalf("restored checkpoint = %+v", paused)
	}
	if _, err := restored.UpdateJobFromClient(paused.ID, sftp.UpdateTransferJob{Action: sftp.TransferResumeAction}); err != nil {
		t.Fatal(err)
	}
	awaitRecoveryStatus(t, restored, sftp.TransferCompleted)
	if target.writes.Load() != 18 || string(target.nodes["/target"].content) != "persistent payload" {
		t.Fatal("restored checkpoint did not resume the suffix")
	}
}
