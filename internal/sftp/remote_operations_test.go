package sftp_test

import (
	"context"
	"errors"
	"io/fs"
	"strconv"
	"strings"
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
	if err := service.DeleteTreeForTest(context.Background(), "edge", "/work"); err != nil {
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
	if err := service.DeleteTreeForTest(context.Background(), "edge", "/"); !errors.Is(err, sftp.ErrRootOperation) {
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
	if err := service.DeleteTreeForTest(context.Background(), "edge", "/work"); !errors.Is(err, sftp.ErrConflict) {
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
	manager := newTestTransferManager(t, service)
	defer manager.Close()
	_, err := manager.CreateJob(sftp.CreateTransferJob{
		ID: "delete_remote_01", BatchID: "delete_batch_01", Alias: "edge", RemotePath: "/work",
		SourceAlias: "edge", SourcePath: "/work", Operation: sftp.RemoteDelete,
		Direction: sftp.TransferRemote, Kind: sftp.TransferFolder, Name: "work", TotalBytes: -1,
	})
	if err != nil {
		t.Fatal(err)
	}
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

func TestQueuedDeleteOfATreeDeeperThanTheLimitFailsAsTraversalLimitAndRemovesNothing(t *testing.T) {
	nodes := map[string]node{"/work": directory("work")}
	deepest := "/work"
	for range sftp.MaxDeleteDepthForTest + 1 {
		deepest += "/d"
		nodes[deepest] = directory("d")
	}
	remote := remoteWith(nodes)
	service := &sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return fakeConnection{remote}, nil }}
	manager := newTestTransferManager(t, service)
	defer manager.Close()
	const id = "delete_too_deep"
	if _, err := manager.CreateJob(sftp.CreateTransferJob{
		ID: id, BatchID: "batch_" + id, Alias: "edge", RemotePath: "/work",
		SourceAlias: "edge", SourcePath: "/work", Operation: sftp.RemoteDelete,
		Direction: sftp.TransferRemote, Kind: sftp.TransferFolder, Name: "work", TotalBytes: -1,
	}); err != nil {
		t.Fatal(err)
	}

	job := waitForJob(t, manager, id, hasStatus(sftp.TransferFailed))
	if job.Problem != "sftp_traversal_limit" {
		t.Fatalf("problem = %q, want sftp_traversal_limit", job.Problem)
	}
	if len(remote.removals) != 0 {
		t.Fatalf("removed entries of a tree it refused: %v", remote.removals)
	}
}

func TestQueuedCopyOfAFolderWithMoreEntriesThanTheLimitFailsAsTraversalLimit(t *testing.T) {
	nodes := map[string]node{"/data": directory("data")}
	for index := range sftp.MaxTransferTreeEntriesForTest + 1 {
		name := "f" + strconv.Itoa(index)
		nodes["/data/"+name] = file(name, "", 0o644)
	}
	source := remoteWith(nodes)
	target := remoteWith(nil)
	manager := newCopyJobManager(t, source, target)
	defer manager.Close()
	const id = "copy_too_many_entries"
	if _, err := manager.CreateJob(sftp.CreateTransferJob{
		ID: id, BatchID: "batch_" + id, Alias: "target", RemotePath: "/data",
		SourceAlias: "source", SourcePath: "/data", Operation: sftp.RemoteCopy,
		Direction: sftp.TransferRemote, Kind: sftp.TransferFolder, Name: "data", TotalBytes: -1,
	}); err != nil {
		t.Fatal(err)
	}

	job := waitForJob(t, manager, id, hasStatus(sftp.TransferFailed))
	if job.Problem != "sftp_traversal_limit" {
		t.Fatalf("problem = %q, want sftp_traversal_limit", job.Problem)
	}
	if _, exists := target.nodes["/data"]; exists {
		t.Fatal("the copy wrote to the target before refusing the tree")
	}
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
	comparison, err := service.CompareDirectories(context.Background(), sftp.CompareOptions{Left: sftp.ComparisonLocation{Alias: "left", Path: "/work"}, Right: sftp.ComparisonLocation{Alias: "right", Path: "/copy"}})
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

// hostileListingRemote は、trap directory の最初の ReadDir だけに server が返した
// 名前として inject を混ぜる。pkg/sftp は "x/.." を path.Base で ".." に縮めるので、
// 悪性 server はこの形で親 directory の外を指せる。
type hostileListingRemote struct {
	*fakeRemote
	trap      string
	inject    string
	triggered bool
}

func (r *hostileListingRemote) ReadDir(ctx context.Context, directory string) ([]fs.FileInfo, error) {
	infos, err := r.fakeRemote.ReadDir(ctx, directory)
	if err != nil {
		return nil, err
	}
	if directory == r.trap && !r.triggered {
		r.triggered = true
		infos = append([]fs.FileInfo{node{name: r.inject, mode: fs.ModeDir | 0o755, modTime: testTime}}, infos...)
	}
	return infos, nil
}

func TestCopyRemoteRefusesEntryNamesThatEscapeTheSourceDirectory(t *testing.T) {
	t.Parallel()
	source := &hostileListingRemote{fakeRemote: remoteWith(map[string]node{
		"/src":            directory("src"),
		"/src/proj":       directory("proj"),
		"/src/proj/a.txt": file("a.txt", "a", 0o644),
		"/src/evil":       file("evil", "pwned", 0o644),
	}), trap: "/src/proj", inject: ".."}
	target := remoteWith(map[string]node{
		"/dst":      directory("dst"),
		"/dst/proj": directory("proj"),
	})
	service := sftp.Service{Open: func(_ context.Context, alias string) (sftp.Remote, error) {
		if alias == "hostile" {
			return source, nil
		}
		return target, nil
	}}
	err := service.CopyRemote(context.Background(), sftp.RemoteTransferRequest{
		SourceAlias: "hostile", SourcePath: "/src/proj",
		TargetAlias: "trusted", TargetPath: "/dst/proj",
		Operation: sftp.RemoteCopy, Overwrite: true,
	}, nil)
	if !errors.Is(err, sftp.ErrInvalidPath) {
		t.Fatalf("CopyRemote() = %v, want ErrInvalidPath", err)
	}
	if _, escaped := target.nodes["/dst/evil"]; escaped {
		t.Fatal("hostile source wrote outside the target directory")
	}
}

func TestListingRejectsEveryServerNameThatLeavesTheDirectory(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"", ".", "..", "a/b", "nul\x00"} {
		remote := &hostileListingRemote{fakeRemote: remoteWith(map[string]node{"/work": directory("work")}), trap: "/work", inject: name}
		service := sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }}
		if _, err := service.ListDirectory(context.Background(), "edge", "/work"); !errors.Is(err, sftp.ErrInvalidPath) {
			t.Fatalf("List() with server name %q = %v, want ErrInvalidPath", name, err)
		}
	}
}

func TestRemoteMoveKeepsASourceFileThatChangedAfterItWasCopied(t *testing.T) {
	t.Parallel()
	source := remoteWith(map[string]node{
		"/data":          directory("data"),
		"/data/keep.txt": file("keep.txt", "before", 0o644),
	})
	target := remoteWith(map[string]node{"/inbox": directory("inbox")})
	// The copy has already verified the source and is publishing its copy when
	// another client rewrites the source. The move must not delete that rewrite.
	target.renameHook = func() {
		source.nodes["/data/keep.txt"] = withTime(file("keep.txt", "after", 0o644), testTime.Add(time.Minute))
	}
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
	if got := string(source.nodes["/data/keep.txt"].content); got != "after" {
		t.Fatalf("source after move = %q, want the rewrite to survive", got)
	}
	if got := string(target.nodes["/inbox/data/keep.txt"].content); got != "before" {
		t.Fatalf("copied target = %q", got)
	}
}

func TestSameHostFolderMoveMergesIntoAnExistingFolderOnceOverwriteIsApproved(t *testing.T) {
	remote := remoteWith(map[string]node{
		"/incoming":                  directory("incoming"),
		"/incoming/new.txt":          file("new.txt", "new", 0o644),
		"/incoming/same.txt":         file("same.txt", "fresh", 0o644),
		"/incoming/nested":           directory("nested"),
		"/incoming/nested/n.txt":     file("n.txt", "nested", 0o644),
		"/incoming/sub":              directory("sub"),
		"/incoming/sub/s.txt":        file("s.txt", "sub", 0o644),
		"/archive":                   directory("archive"),
		"/archive/incoming":          directory("incoming"),
		"/archive/incoming/old.txt":  file("old.txt", "old", 0o644),
		"/archive/incoming/same.txt": file("same.txt", "stale", 0o644),
		"/archive/incoming/nested":   directory("nested"),
	})
	service := sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return fakeConnection{remote}, nil }}
	request := sftp.RemoteTransferRequest{
		SourceAlias: "edge", SourcePath: "/incoming", TargetAlias: "edge", TargetPath: "/archive/incoming",
		Operation: sftp.RemoteMove,
	}
	if err := service.CopyRemote(context.Background(), request, nil); !errors.Is(err, sftp.ErrAlreadyExists) {
		t.Fatalf("move without approval: err = %v", err)
	}
	request.Overwrite = true
	if err := service.CopyRemote(context.Background(), request, nil); err != nil {
		t.Fatalf("approved move: %v", err)
	}
	for candidate, contents := range map[string]string{
		"/archive/incoming/old.txt":      "old",
		"/archive/incoming/same.txt":     "fresh",
		"/archive/incoming/new.txt":      "new",
		"/archive/incoming/nested/n.txt": "nested",
		"/archive/incoming/sub/s.txt":    "sub",
	} {
		if got := string(remote.nodes[candidate].content); got != contents {
			t.Fatalf("%s = %q, want %q", candidate, got, contents)
		}
	}
	for candidate := range remote.nodes {
		if candidate == "/incoming" || strings.HasPrefix(candidate, "/incoming/") {
			t.Fatalf("source entry %s is left after the move", candidate)
		}
	}
}

func TestCopyOfAFolderWithoutOwnerWriteWritesItsChildrenAndKeepsItsMode(t *testing.T) {
	source := remoteWith(map[string]node{
		"/mod":           {name: "mod", mode: fs.ModeDir | 0o555, modTime: testTime},
		"/mod/go.mod":    file("go.mod", "module example", 0o444),
		"/mod/sub":       {name: "sub", mode: fs.ModeDir | 0o555, modTime: testTime},
		"/mod/sub/a.txt": file("a.txt", "nested", 0o444),
	})
	target := remoteWith(map[string]node{"/inbox": directory("inbox")})
	service := sftp.Service{Open: func(_ context.Context, alias string) (sftp.Remote, error) {
		if alias == "source" {
			return fakeConnection{source}, nil
		}
		return fakeConnection{target}, nil
	}}
	if err := service.CopyRemote(context.Background(), sftp.RemoteTransferRequest{
		SourceAlias: "source", SourcePath: "/mod", TargetAlias: "target", TargetPath: "/inbox/mod",
		Operation: sftp.RemoteCopy,
	}, nil); err != nil {
		t.Fatalf("copy: %v", err)
	}
	if got := string(target.nodes["/inbox/mod/sub/a.txt"].content); got != "nested" {
		t.Fatalf("nested file = %q", got)
	}
	for _, folder := range []string{"/inbox/mod", "/inbox/mod/sub"} {
		if mode := target.nodes[folder].mode; !mode.IsDir() || mode.Perm() != 0o555 {
			t.Fatalf("%s mode = %v, want the source's dr-xr-xr-x", folder, mode)
		}
	}
}

func TestAnApprovedOverwriteOfAFileWithAFolderFailsAsAConflictInsteadOfAskingAgain(t *testing.T) {
	folder := map[string]node{"/data": directory("data"), "/data/a.txt": file("a.txt", "folder child", 0o644)}
	fileInTheWay := map[string]node{"/inbox": directory("inbox"), "/inbox/data": file("data", "file", 0o644)}
	sameHost := remoteWith(map[string]node{})
	for name, entry := range folder {
		sameHost.nodes[name] = entry
	}
	for name, entry := range fileInTheWay {
		sameHost.nodes[name] = entry
	}
	sameHostService := sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return fakeConnection{sameHost}, nil }}
	otherHostTarget := remoteWith(fileInTheWay)
	for _, run := range []struct {
		name    string
		service sftp.Service
		target  *fakeRemote
		request sftp.RemoteTransferRequest
	}{
		{
			name: "move on one host", service: sameHostService, target: sameHost,
			request: sftp.RemoteTransferRequest{SourceAlias: "edge", SourcePath: "/data", TargetAlias: "edge", TargetPath: "/inbox/data", Operation: sftp.RemoteMove, Overwrite: true},
		},
		{
			name: "copy between hosts", service: twoHostService(fakeConnection{remoteWith(folder)}, fakeConnection{otherHostTarget}), target: otherHostTarget,
			request: sftp.RemoteTransferRequest{SourceAlias: "source", SourcePath: "/data", TargetAlias: "target", TargetPath: "/inbox/data", Operation: sftp.RemoteCopy, Overwrite: true},
		},
	} {
		t.Run(run.name, func(t *testing.T) {
			err := run.service.CopyRemote(context.Background(), run.request, nil)
			if !errors.Is(err, sftp.ErrConflict) || errors.Is(err, sftp.ErrAlreadyExists) {
				t.Fatalf("err = %v, want a conflict rather than another overwrite question", err)
			}
			if got := string(run.target.nodes["/inbox/data"].content); got != "file" {
				t.Fatalf("the file in the way = %q, want it untouched", got)
			}
		})
	}
}
