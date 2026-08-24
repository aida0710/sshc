package sftp_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"sshc/internal/sftp"
)

func TestDownloadFromReturnsOnlyRemainingBytes(t *testing.T) {
	remote := remoteWith(map[string]node{"/large.bin": file("large.bin", "abcdefgh", 0o600)})
	service := serviceFor(remote)
	var output bytes.Buffer
	transfer, err := service.DownloadFrom(t.Context(), "edge", "/large.bin", 3, &output)
	if err != nil {
		t.Fatal(err)
	}
	if output.String() != "defgh" || transfer.Bytes != 5 {
		t.Fatalf("download = %q / %d", output.String(), transfer.Bytes)
	}
	if _, err := service.DownloadFrom(t.Context(), "edge", "/large.bin", 9, &output); !errors.Is(err, sftp.ErrOffsetMismatch) {
		t.Fatalf("past-end offset = %v", err)
	}
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
	completed, err := manager.Complete(t.Context(), "edge", started.ID, started.Path, 9, sftp.AbsentRevision)
	if err != nil || completed.Bytes != 9 {
		t.Fatalf("Complete() = %+v, %v", completed, err)
	}
	if got := string(remote.nodes["/remote/large.bin"].content); got != "largefile" {
		t.Fatalf("target contents = %q", got)
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
	changed.content = []byte("external")
	changed.modTime = changed.modTime.Add(1)
	remote.nodes["/remote/existing.txt"] = changed
	if _, err := manager.Complete(t.Context(), "edge", started.ID, started.Path, 3, started.ExpectedRevision); !errors.Is(err, sftp.ErrConflict) {
		t.Fatalf("Complete(stale) = %v", err)
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
	if _, err := manager.Complete(t.Context(), "edge", first.ID, first.Path, 3, first.ExpectedRevision); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Complete(t.Context(), "edge", second.ID, second.Path, 3, second.ExpectedRevision); !errors.Is(err, sftp.ErrAlreadyExists) {
		t.Fatalf("second Complete() = %v", err)
	}
}
