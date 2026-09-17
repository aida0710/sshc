package sftp_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sshc/internal/sftp"
)

func TestLocalListingStartsAtEngineHomeAndCanNavigateAboveIt(t *testing.T) {
	parent := t.TempDir()
	home := filepath.Join(parent, "home")
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.WriteFile(filepath.Join(home, "note.txt"), []byte("local"), 0600); err != nil {
		t.Fatal(err)
	}
	listing, err := sftp.ListLocal("")
	if err != nil {
		t.Fatal(err)
	}
	if listing.Path != filepath.ToSlash(home) || listing.Home != filepath.ToSlash(home) || len(listing.Entries) != 1 || listing.Entries[0].Name != "note.txt" {
		t.Fatalf("listing = %+v", listing)
	}
	// The local list shows the same columns as a remote one, so every entry
	// carries the metadata the shared table renders.
	note := listing.Entries[0]
	info, err := os.Stat(filepath.Join(home, "note.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if note.Type != sftp.EntryFile || note.Size != 5 || note.Mode != info.Mode() || !note.ModifiedAt.Equal(info.ModTime()) ||
		note.ModifiedAt.Location() != time.UTC || !strings.HasPrefix(note.Revision, "meta-sha256:") {
		t.Fatalf("note entry = %+v (stat %v %v)", note, info.Mode(), info.ModTime())
	}
	above, err := sftp.ListLocal(parent)
	if err != nil {
		t.Fatal(err)
	}
	if above.Path != filepath.ToSlash(parent) || len(above.Entries) != 1 || above.Entries[0].Name != "home" {
		t.Fatalf("parent listing = %+v", above)
	}
	if _, err := sftp.ListLocal("relative/path"); !errors.Is(err, sftp.ErrInvalidPath) {
		t.Fatalf("relative path error = %v", err)
	}
	withTrailingSlash, err := sftp.ListLocal(filepath.ToSlash(home) + "/")
	if err != nil || withTrailingSlash.Path != filepath.ToSlash(home) {
		t.Fatalf("trailing slash listing = %+v, %v", withTrailingSlash, err)
	}
}

func TestEngineLocalTransferStreamsBothDirections(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	source := filepath.Join(home, "local.txt")
	if err := os.WriteFile(source, []byte("local payload"), 0600); err != nil {
		t.Fatal(err)
	}
	remote := remoteWith(map[string]node{
		"/inbox":            {name: "inbox", mode: fs.ModeDir | 0755, modTime: testTime},
		"/inbox/remote.txt": file("remote.txt", "remote payload", 0644),
	})
	service := sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }, TemporaryPath: func(candidate string) (string, error) { return candidate + ".part", nil }}
	var uploaded int64
	put := sftp.RemoteTransferRequest{SourceAlias: "edge", SourcePath: source, TargetAlias: "edge", TargetPath: "/inbox/local.txt", Operation: sftp.RemotePut}
	if err := service.CopyLocal(context.Background(), put, func(n int64) error { uploaded = n; return nil }); err != nil {
		t.Fatal(err)
	}
	if string(remote.nodes["/inbox/local.txt"].content) != "local payload" || uploaded != 13 {
		t.Fatalf("upload contents/progress = %q/%d", remote.nodes["/inbox/local.txt"].content, uploaded)
	}
	target := filepath.Join(home, "downloaded.txt")
	get := sftp.RemoteTransferRequest{SourceAlias: "edge", SourcePath: "/inbox/remote.txt", TargetAlias: "edge", TargetPath: target, Operation: sftp.RemoteGet}
	if err := service.CopyLocal(context.Background(), get, nil); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "remote payload" {
		t.Fatalf("download contents = %q", contents)
	}
	if err := service.CopyLocal(context.Background(), get, nil); !errors.Is(err, sftp.ErrAlreadyExists) {
		t.Fatalf("overwrite protection = %v", err)
	}
	if err := os.WriteFile(target, []byte("old payload"), 0600); err != nil {
		t.Fatal(err)
	}
	get.Overwrite = true
	if err := service.CopyLocal(context.Background(), get, nil); err != nil {
		t.Fatalf("overwrite = %v", err)
	}
	contents, err = os.ReadFile(target)
	if err != nil || string(contents) != "remote payload" {
		t.Fatalf("overwrite contents = %q, %v", contents, err)
	}
}

func TestQueuedEngineLocalDownloadCompletes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	remote := remoteWith(map[string]node{
		"/remote":            {name: "remote", mode: fs.ModeDir | 0755, modTime: testTime},
		"/remote/queued.txt": file("queued.txt", "queued payload", 0644),
	})
	service := &sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }}
	manager := sftp.NewTransferManager(service)
	defer manager.Close()
	target := filepath.Join(home, "queued.txt")
	job, err := manager.CreateJob(sftp.CreateTransferJob{
		ID: "local_get_01", BatchID: "local_batch_01", Alias: "edge", SourceAlias: "edge", SourcePath: "/remote/queued.txt", RemotePath: target,
		Direction: sftp.TransferRemote, Operation: sftp.RemoteGet, Kind: sftp.TransferFile, Name: "queued.txt", TotalBytes: -1,
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
			contents, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			if string(contents) != "queued payload" || jobs[0].TransferredBytes != 14 {
				t.Fatalf("queue result = %+v, %q", jobs[0], contents)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("queued local download did not complete")
}

func TestLocalDownloadRejectsRemoteTraversalName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	remote := remoteWith(map[string]node{
		"/remote":    {name: "remote", mode: fs.ModeDir | 0755, modTime: testTime},
		"/remote/..": {name: "..", mode: fs.ModeDir | 0755, modTime: testTime},
	})
	service := sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }}
	err := service.CopyLocal(context.Background(), sftp.RemoteTransferRequest{
		SourceAlias: "edge", SourcePath: "/remote", TargetAlias: "edge", TargetPath: filepath.Join(home, "download"), Operation: sftp.RemoteGet,
	}, nil)
	if !errors.Is(err, sftp.ErrInvalidPath) {
		t.Fatalf("traversal = %v", err)
	}
}

func TestLocalDownloadRejectsEmptyRemoteName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	remote := remoteWith(map[string]node{
		"/remote":       {name: "remote", mode: fs.ModeDir | 0755, modTime: testTime},
		"/remote/empty": {name: "", mode: fs.ModeDir | 0755, modTime: testTime},
	})
	service := sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }}
	err := service.CopyLocal(context.Background(), sftp.RemoteTransferRequest{
		SourceAlias: "edge", SourcePath: "/remote", TargetAlias: "edge", TargetPath: filepath.Join(home, "download"), Operation: sftp.RemoteGet,
	}, nil)
	if !errors.Is(err, sftp.ErrInvalidPath) {
		t.Fatalf("empty remote name = %v", err)
	}
}
