package sftp_test

import (
	"context"
	"errors"
	"io/fs"
	"testing"
	"time"

	"sshc/internal/sftp"
)

func TestDeleteDirectoryRecursivelyWithoutFollowingSymlinks(t *testing.T) {
	remote := remoteWith(map[string]node{
		"/work":                 {name: "work", mode: fs.ModeDir | 0o755},
		"/work/nested":          {name: "nested", mode: fs.ModeDir | 0o755},
		"/work/nested/file.txt": file("file.txt", "payload", 0o644),
		"/work/link":            {name: "link", mode: fs.ModeSymlink | 0o777},
		"/outside":              file("outside", "keep", 0o644),
	})
	service := sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }}
	if err := service.Delete(context.Background(), "edge", "/work"); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []string{"/work", "/work/nested", "/work/nested/file.txt", "/work/link"} {
		if _, exists := remote.nodes[candidate]; exists {
			t.Errorf("%s was not removed", candidate)
		}
	}
	if _, exists := remote.nodes["/outside"]; !exists {
		t.Fatal("symlink target was removed")
	}
	if err := service.Delete(context.Background(), "edge", "/"); !errors.Is(err, sftp.ErrRootOperation) {
		t.Fatalf("root deletion = %v", err)
	}
}

func TestDeleteDirectoryRejectsActiveInternalEntryBeforeRemovingAnything(t *testing.T) {
	remote := remoteWith(map[string]node{
		"/work":                                 {name: "work", mode: fs.ModeDir | 0o755},
		"/work/a.txt":                           file("a.txt", "keep", 0o644),
		"/work/.file.sshc-upload-12345678.part": file(".file.sshc-upload-12345678.part", "in progress", 0o600),
	})
	service := sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }}
	if err := service.Delete(context.Background(), "edge", "/work"); !errors.Is(err, sftp.ErrConflict) {
		t.Fatalf("delete with active internal entry = %v", err)
	}
	if len(remote.removals) != 0 {
		t.Fatalf("deleted entries before validating the tree: %v", remote.removals)
	}
}

func TestQueuedDirectoryDeleteCompletes(t *testing.T) {
	remote := remoteWith(map[string]node{
		"/work":          {name: "work", mode: fs.ModeDir | 0o755},
		"/work/file.txt": file("file.txt", "payload", 0o644),
	})
	service := &sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }}
	manager := sftp.NewTransferManager(service)
	defer manager.Close()
	job, err := manager.CreateJob(sftp.CreateTransferJob{
		ID: "delete_remote_01", BatchID: "delete_batch_01", Alias: "edge", RemotePath: "/work",
		SourceAlias: "edge", SourcePath: "/work", Operation: sftp.RemoteDelete,
		Direction: sftp.TransferRemote, Kind: sftp.TransferFolder, Name: "work", TotalBytes: -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	manager.ScheduleRemoteJob(job.ID)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		jobs, err := manager.ListJobs()
		if err != nil {
			t.Fatal(err)
		}
		if len(jobs) == 1 && jobs[0].Status == sftp.TransferCompleted {
			if jobs[0].TotalBytes != 2 || jobs[0].TransferredBytes != 2 {
				t.Fatalf("delete progress = %+v", jobs[0])
			}
			if _, exists := remote.nodes["/work"]; exists {
				t.Fatal("directory remained")
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("queued delete did not complete")
}

func TestCopyRemoteStreamsFileWithoutLocalSpool(t *testing.T) {
	t.Parallel()
	source := remoteWith(map[string]node{
		"/data":          {name: "data", mode: fs.ModeDir | 0o750, modTime: testTime},
		"/data/file.txt": {name: "file.txt", mode: 0o640, content: []byte("direct stream"), modTime: testTime},
	})
	target := remoteWith(map[string]node{
		"/inbox": {name: "inbox", mode: fs.ModeDir | 0o755, modTime: testTime},
	})
	service := sftp.Service{
		Open: func(_ context.Context, alias string) (sftp.Remote, error) {
			if alias == "source" {
				return source, nil
			}
			return target, nil
		},
		TemporaryPath: func(candidate string) (string, error) { return candidate + ".part", nil },
	}
	var progress int64
	err := service.CopyRemote(context.Background(), sftp.RemoteTransferRequest{
		SourceAlias: "source", SourcePath: "/data/file.txt",
		TargetAlias: "target", TargetPath: "/inbox/file.txt", Operation: sftp.RemoteCopy,
	}, func(total int64) error { progress = total; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if got := string(target.nodes["/inbox/file.txt"].content); got != "direct stream" {
		t.Fatalf("copied contents = %q", got)
	}
	if target.nodes["/inbox/file.txt"].mode.Perm() != 0o640 {
		t.Fatalf("copied mode = %o", target.nodes["/inbox/file.txt"].mode.Perm())
	}
	if progress != int64(len("direct stream")) {
		t.Fatalf("progress = %d", progress)
	}
	if _, exists := target.nodes["/inbox/file.txt.part"]; exists {
		t.Fatal("temporary target remained after atomic rename")
	}
}

func TestRemoteMoveRejectsUncopiedInternalEntryBeforeWriting(t *testing.T) {
	t.Parallel()
	source := remoteWith(map[string]node{
		"/data": {name: "data", mode: fs.ModeDir | 0o750, modTime: testTime},
		"/data/.file.sshc-upload-12345678.part": {
			name: ".file.sshc-upload-12345678.part", mode: 0o600, content: []byte("in progress"), modTime: testTime,
		},
	})
	target := remoteWith(map[string]node{
		"/inbox": {name: "inbox", mode: fs.ModeDir | 0o755, modTime: testTime},
	})
	service := sftp.Service{
		Open: func(_ context.Context, alias string) (sftp.Remote, error) {
			if alias == "source" {
				return source, nil
			}
			return target, nil
		},
		TemporaryPath: func(candidate string) (string, error) { return candidate + ".part", nil },
	}
	err := service.CopyRemote(context.Background(), sftp.RemoteTransferRequest{
		SourceAlias: "source", SourcePath: "/data",
		TargetAlias: "target", TargetPath: "/inbox/data", Operation: sftp.RemoteMove,
	}, nil)
	if !errors.Is(err, sftp.ErrConflict) {
		t.Fatalf("move error = %v, want conflict", err)
	}
	if _, exists := target.nodes["/inbox/data"]; exists {
		t.Fatal("target was modified before the move tree was validated")
	}
	if _, exists := source.nodes["/data/.file.sshc-upload-12345678.part"]; !exists {
		t.Fatal("uncopied source entry was removed")
	}
}

func TestCompareDirectoriesReportsBothSidesAndChangedMetadata(t *testing.T) {
	t.Parallel()
	left := remoteWith(map[string]node{
		"/work":         {name: "work", mode: fs.ModeDir | 0o755, modTime: testTime},
		"/work/same":    {name: "same", mode: 0o600, content: []byte("same"), modTime: testTime},
		"/work/changed": {name: "changed", mode: 0o600, content: []byte("left"), modTime: testTime},
		"/work/left":    {name: "left", mode: 0o600, content: []byte("only"), modTime: testTime},
	})
	right := remoteWith(map[string]node{
		"/copy":         {name: "copy", mode: fs.ModeDir | 0o755, modTime: testTime},
		"/copy/same":    {name: "same", mode: 0o600, content: []byte("same"), modTime: testTime},
		"/copy/changed": {name: "changed", mode: 0o600, content: []byte("right-longer"), modTime: testTime},
		"/copy/right":   {name: "right", mode: 0o600, content: []byte("only"), modTime: testTime},
	})
	service := sftp.Service{Open: func(_ context.Context, alias string) (sftp.Remote, error) {
		if alias == "left" {
			return left, nil
		}
		return right, nil
	}}
	comparison, err := service.CompareDirectories(context.Background(), "left", "/work", "right", "/copy")
	if err != nil {
		t.Fatal(err)
	}
	statuses := map[string]sftp.DirectoryDifferenceStatus{}
	for _, entry := range comparison.Entries {
		statuses[entry.RelativePath] = entry.Status
	}
	if statuses["same"] != sftp.DirectorySame || statuses["changed"] != sftp.DirectoryDifferent ||
		statuses["left"] != sftp.DirectoryLeftOnly || statuses["right"] != sftp.DirectoryRightOnly {
		t.Fatalf("unexpected comparison: %#v", statuses)
	}
}

func TestRemoteDirectoryCannotBeCopiedIntoItself(t *testing.T) {
	t.Parallel()
	remote := remoteWith(map[string]node{
		"/source":          {name: "source", mode: fs.ModeDir | 0o755, modTime: testTime},
		"/source/file.txt": {name: "file.txt", mode: 0o644, content: []byte("contents"), modTime: testTime},
	})
	service := sftp.Service{Open: func(_ context.Context, alias string) (sftp.Remote, error) {
		if alias != "same" {
			t.Fatalf("unexpected alias %q", alias)
		}
		return remote, nil
	}}

	err := service.CopyRemote(context.Background(), sftp.RemoteTransferRequest{
		SourceAlias: "same", SourcePath: "/source",
		TargetAlias: "same", TargetPath: "/source/nested", Operation: sftp.RemoteCopy,
	}, nil)
	if !errors.Is(err, sftp.ErrInvalidTransfer) {
		t.Fatalf("CopyRemote() error = %v, want ErrInvalidTransfer", err)
	}
	if _, exists := remote.nodes["/source/nested"]; exists {
		t.Fatal("target was created below its own source")
	}
}
