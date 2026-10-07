package sftp

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// Allow the configured reconnect backoff and fresh OpenSSH process startup.
const opensshRecoveryTimeout = 10 * time.Second

// Job state has no test subscription; poll without occupying the worker.
const opensshRecoveryPollInterval = 10 * time.Millisecond

type interruptedOpenSSHRemote struct {
	*Client
	interrupt *atomic.Bool
	written   *atomic.Int64
}

func (remote *interruptedOpenSSHRemote) OpenFile(name string, flags int) (WriteSeekCloser, error) {
	file, err := remote.Client.OpenFile(name, flags)
	if err != nil {
		return nil, err
	}
	return &interruptedOpenSSHWriter{WriteSeekCloser: file, remote: remote}, nil
}

type interruptedOpenSSHWriter struct {
	WriteSeekCloser
	remote *interruptedOpenSSHRemote
}

func (writer *interruptedOpenSSHWriter) Write(contents []byte) (int, error) {
	if writer.remote.interrupt.CompareAndSwap(true, false) {
		written, err := writer.WriteSeekCloser.Write(contents[:len(contents)/2])
		writer.remote.written.Add(int64(written))
		if err != nil {
			return written, err
		}
		// The real server retains the prefix, but this protocol connection ends.
		_ = writer.remote.Client.Close()
		return written, io.EOF
	}
	written, err := writer.WriteSeekCloser.Write(contents)
	writer.remote.written.Add(int64(written))
	return written, err
}

func TestOpenSSHDisconnectedCopyReconnectsAndSendsOnlyTheVerifiedSuffix(t *testing.T) {
	// Check fixture availability before launching a manager goroutine.
	probe := openOpenSSHTestClient(t)
	_ = probe.Close()
	directory := t.TempDir()
	source := filepath.Join(directory, "source.txt")
	target := filepath.Join(directory, "target.txt")
	contents := bytes.Repeat([]byte("verified OpenSSH payload\n"), 100)
	if err := os.WriteFile(source, contents, 0o640); err != nil {
		t.Fatal(err)
	}
	var interrupt atomic.Bool
	interrupt.Store(true)
	var written atomic.Int64
	var opened atomic.Int32
	service := &Service{Open: func(context.Context, string) (Remote, error) {
		opened.Add(1)
		return &interruptedOpenSSHRemote{Client: openOpenSSHTestClient(t), interrupt: &interrupt, written: &written}, nil
	}}
	manager := NewTransferManager(service, t.TempDir())
	defer func() { _ = manager.Close() }()
	settings := DefaultTransferSettings()
	settings.AutoReconnect, settings.MaxReconnectAttempts = true, 2
	if err := manager.SetTransferSettings(settings); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.CreateJob(CreateTransferJob{
		ID: "openssh_recovery", BatchID: "openssh_batch", BatchName: "OpenSSH copy", Name: "source.txt",
		Alias: "fixture", SourceAlias: "fixture", SourcePath: filepath.ToSlash(source), RemotePath: filepath.ToSlash(target),
		Operation: RemoteCopy, Direction: TransferRemote, Kind: TransferFile, BatchKind: TransferFile, TotalBytes: int64(len(contents)),
	}); err != nil {
		t.Fatal(err)
	}
	deadline := time.NewTimer(opensshRecoveryTimeout)
	defer deadline.Stop()
	ticks := time.NewTicker(opensshRecoveryPollInterval)
	defer ticks.Stop()
	for {
		jobs, err := manager.ListJobs()
		if err != nil {
			t.Fatal(err)
		}
		if len(jobs) == 1 && jobs[0].Status == TransferCompleted {
			if jobs[0].ReconnectAttempt != 1 || opened.Load() < 2 || written.Load() != int64(len(contents)) {
				t.Fatalf("recovery = %+v; connections = %d, bytes sent = %d", jobs[0], opened.Load(), written.Load())
			}
			copied, err := os.ReadFile(target)
			if err != nil || !bytes.Equal(copied, contents) {
				t.Fatalf("recovered target differs: %v", err)
			}
			return
		}
		if len(jobs) == 1 && jobs[0].Status == TransferFailed {
			t.Fatalf("OpenSSH recovery failed: %+v", jobs[0])
		}
		select {
		case <-deadline.C:
			t.Fatalf("OpenSSH recovery did not complete: %+v", jobs)
		case <-ticks.C:
		}
	}
}
