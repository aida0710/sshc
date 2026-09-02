package sftp_test

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	"sshc/internal/sftp"
)

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
