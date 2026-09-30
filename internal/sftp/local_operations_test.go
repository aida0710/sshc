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
	manager := newTestTransferManager(t, service)
	defer manager.Close()
	target := filepath.Join(home, "queued.txt")
	_, err := manager.CreateJob(sftp.CreateTransferJob{
		ID: "local_get_01", BatchID: "local_batch_01", Alias: "edge", SourceAlias: "edge", SourcePath: "/remote/queued.txt", RemotePath: target,
		Direction: sftp.TransferRemote, Operation: sftp.RemoteGet, Kind: sftp.TransferFile, Name: "queued.txt", TotalBytes: -1,
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

// A Windows engine opens os.Root at the volume root and a zip is unpacked on
// whatever machine the browser runs on, so a backslash in a remote name must
// be refused on every OS: nothing else keeps "..\..\evil" inside the target.
func TestRemoteNamesWithABackslashAreRefusedForLocalDownloadsAndZips(t *testing.T) {
	for _, name := range []string{`..\..\evil`, `a\b`} {
		remote := func() *fakeRemote {
			return remoteWith(map[string]node{
				"/remote":         directory("remote"),
				"/remote/" + name: file(name, "payload", 0o600),
			})
		}
		t.Run("get "+name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			err := serviceFor(remote()).CopyLocal(context.Background(), sftp.RemoteTransferRequest{
				SourceAlias: "edge", SourcePath: "/remote", TargetAlias: "edge", TargetPath: filepath.Join(home, "download"), Operation: sftp.RemoteGet,
			}, nil)
			if !errors.Is(err, sftp.ErrInvalidPath) {
				t.Fatalf("local download of %q = %v, want ErrInvalidPath", name, err)
			}
		})
		t.Run("zip child "+name, func(t *testing.T) {
			var archive strings.Builder
			if _, err := serviceFor(remote()).DownloadArchive(context.Background(), "edge", "/remote", &archive); !errors.Is(err, sftp.ErrInvalidPath) {
				t.Fatalf("zip with child %q = %v, want ErrInvalidPath", name, err)
			}
		})
	}
	t.Run("zip root", func(t *testing.T) {
		remote := remoteWith(map[string]node{
			`/a\b`:        directory(`a\b`),
			`/a\b/inside`: file("inside", "payload", 0o600),
		})
		var archive strings.Builder
		if _, err := serviceFor(remote).DownloadArchive(context.Background(), "edge", `/a\b`, &archive); !errors.Is(err, sftp.ErrInvalidPath) {
			t.Fatalf(`zip rooted at "a\b" = %v, want ErrInvalidPath`, err)
		}
	})
}

func TestDownloadOfAFolderWithoutOwnerWriteWritesItsChildren(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	remote := remoteWith(map[string]node{
		"/mod":           {name: "mod", mode: fs.ModeDir | 0o555, modTime: testTime},
		"/mod/go.mod":    file("go.mod", "module example", 0o444),
		"/mod/sub":       {name: "sub", mode: fs.ModeDir | 0o555, modTime: testTime},
		"/mod/sub/a.txt": file("a.txt", "nested", 0o444),
	})
	service := sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }}
	target := filepath.Join(home, "mod")
	if err := service.CopyLocal(context.Background(), sftp.RemoteTransferRequest{
		SourceAlias: "edge", SourcePath: "/mod", TargetPath: filepath.ToSlash(target), Operation: sftp.RemoteGet,
	}, nil); err != nil {
		t.Fatalf("get: %v", err)
	}
	contents, err := os.ReadFile(filepath.Join(target, "sub", "a.txt"))
	if err != nil || string(contents) != "nested" {
		t.Fatalf("nested file = %q, %v", contents, err)
	}
	// The downloaded folders stay writable by their owner, so a rerun can
	// replace their children and the user can delete them.
	if err := os.RemoveAll(target); err != nil {
		t.Fatalf("remove the downloaded folder: %v", err)
	}
}

func TestLocalListingShowsLinksAsLinksWithWhatTheyPointTo(t *testing.T) {
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "data.txt"), []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	folder := t.TempDir()
	if err := os.WriteFile(filepath.Join(folder, "note.txt"), []byte("note"), 0600); err != nil {
		t.Fatal(err)
	}
	links := map[string]string{
		"relative-file":   "note.txt",
		"absolute-folder": outside,
		"absolute-file":   filepath.Join(outside, "data.txt"),
		"broken":          filepath.Join(outside, "missing"),
	}
	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(folder, name)); err != nil {
			t.Skipf("symlinks are unavailable: %v", err)
		}
	}

	listing, err := sftp.ListLocal(filepath.ToSlash(folder))
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]sftp.Entry{}
	var order []string
	for _, entry := range listing.Entries {
		byName[entry.Name] = entry
		order = append(order, entry.Name)
	}
	if strings.Join(order, ",") != "absolute-folder,absolute-file,broken,note.txt,relative-file" {
		t.Fatalf("listing order = %q", order)
	}
	for name, wantTarget := range map[string]sftp.LinkTargetType{
		"relative-file": sftp.LinkTargetFile, "absolute-folder": sftp.LinkTargetDirectory,
		"absolute-file": sftp.LinkTargetFile, "broken": "",
	} {
		entry := byName[name]
		if entry.Type != sftp.EntrySymlink || entry.TargetType != wantTarget || entry.LinkTarget != filepath.ToSlash(links[name]) ||
			entry.Path != filepath.ToSlash(filepath.Join(folder, name)) {
			t.Errorf("%s = %+v", name, entry)
		}
	}
	if size := byName["absolute-file"].Size; size != int64(len("outside")) {
		t.Errorf("a link to a file lists the size %d, want the file's", size)
	}

	// Opening a link to a folder lists the folder under the link's path.
	throughLink, err := sftp.ListLocal(filepath.ToSlash(filepath.Join(folder, "absolute-folder")))
	if err != nil {
		t.Fatal(err)
	}
	if throughLink.Path != filepath.ToSlash(filepath.Join(folder, "absolute-folder")) ||
		len(throughLink.Entries) != 1 || throughLink.Entries[0].Path != throughLink.Path+"/data.txt" {
		t.Fatalf("listing through a link = %+v", throughLink)
	}
}

func TestLocalListingOpensAHomeReachedThroughAnAbsoluteLink(t *testing.T) {
	real := t.TempDir()
	if err := os.WriteFile(filepath.Join(real, "note.txt"), []byte("note"), 0600); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(t.TempDir(), "home")
	if err := os.Symlink(real, home); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	listing, err := sftp.ListLocal("")
	if err != nil {
		t.Fatal(err)
	}
	if listing.Path != filepath.ToSlash(home) || len(listing.Entries) != 1 || listing.Entries[0].Name != "note.txt" {
		t.Fatalf("listing = %+v", listing)
	}
}

func TestEngineLocalTransferPassesThroughFolderLinksButRefusesALinkNamedDirectly(t *testing.T) {
	real := t.TempDir()
	if err := os.WriteFile(filepath.Join(real, "local.txt"), []byte("local payload"), 0600); err != nil {
		t.Fatal(err)
	}
	folder := t.TempDir()
	linkedFolder := filepath.Join(folder, "linked")
	if err := os.Symlink(real, linkedFolder); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	if err := os.Symlink(filepath.Join(real, "local.txt"), filepath.Join(folder, "file-link")); err != nil {
		t.Fatal(err)
	}
	remote := remoteWith(map[string]node{
		"/inbox":            {name: "inbox", mode: fs.ModeDir | 0755, modTime: testTime},
		"/inbox/remote.txt": file("remote.txt", "remote payload", 0644),
	})
	service := sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }, TemporaryPath: func(candidate string) (string, error) { return candidate + ".part", nil }}

	put := sftp.RemoteTransferRequest{SourceAlias: "edge", SourcePath: filepath.Join(linkedFolder, "local.txt"), TargetAlias: "edge", TargetPath: "/inbox/local.txt", Operation: sftp.RemotePut}
	if _, err := service.PlanLocalTransfer(context.Background(), put); err != nil {
		t.Fatalf("plan through a folder link = %v", err)
	}
	if err := service.CopyLocal(context.Background(), put, nil); err != nil {
		t.Fatalf("put through a folder link = %v", err)
	}
	if string(remote.nodes["/inbox/local.txt"].content) != "local payload" {
		t.Fatalf("uploaded contents = %q", remote.nodes["/inbox/local.txt"].content)
	}
	get := sftp.RemoteTransferRequest{SourceAlias: "edge", SourcePath: "/inbox/remote.txt", TargetAlias: "edge", TargetPath: filepath.Join(linkedFolder, "remote.txt"), Operation: sftp.RemoteGet}
	if err := service.CopyLocal(context.Background(), get, nil); err != nil {
		t.Fatalf("get through a folder link = %v", err)
	}
	if contents, err := os.ReadFile(filepath.Join(real, "remote.txt")); err != nil || string(contents) != "remote payload" {
		t.Fatalf("downloaded contents = %q, %v", contents, err)
	}

	put.SourcePath = filepath.Join(folder, "file-link")
	if _, err := service.PlanLocalTransfer(context.Background(), put); !errors.Is(err, sftp.ErrUnsupportedEntry) {
		t.Fatalf("plan of a link named directly = %v", err)
	}
}

func TestAnApprovedOverwriteOfAFileWithAFolderFailsAsAConflictOnEitherSide(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	localFolder := filepath.Join(home, "data")
	if err := os.Mkdir(localFolder, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localFolder, "a.txt"), []byte("folder child"), 0o600); err != nil {
		t.Fatal(err)
	}
	localFileInTheWay := filepath.Join(home, "downloaded")
	if err := os.WriteFile(localFileInTheWay, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	remote := remoteWith(map[string]node{
		"/inbox":             directory("inbox"),
		"/inbox/data":        file("data", "file", 0o644),
		"/remote":            directory("remote"),
		"/remote/data":       directory("data"),
		"/remote/data/a.txt": file("a.txt", "folder child", 0o644),
	})
	service := sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return fakeConnection{remote}, nil }}
	for name, request := range map[string]sftp.RemoteTransferRequest{
		"put": {SourceAlias: "edge", SourcePath: filepath.ToSlash(localFolder), TargetAlias: "edge", TargetPath: "/inbox/data", Operation: sftp.RemotePut, Overwrite: true},
		"get": {SourceAlias: "edge", SourcePath: "/remote/data", TargetAlias: "edge", TargetPath: filepath.ToSlash(localFileInTheWay), Operation: sftp.RemoteGet, Overwrite: true},
	} {
		t.Run(name, func(t *testing.T) {
			err := service.CopyLocal(context.Background(), request, nil)
			if !errors.Is(err, sftp.ErrConflict) || errors.Is(err, sftp.ErrAlreadyExists) {
				t.Fatalf("err = %v, want a conflict rather than another overwrite question", err)
			}
		})
	}
	if got := string(remote.nodes["/inbox/data"].content); got != "file" {
		t.Fatalf("the remote file in the way = %q, want it untouched", got)
	}
	if got, err := os.ReadFile(localFileInTheWay); err != nil || string(got) != "file" {
		t.Fatalf("the local file in the way = %q, %v, want it untouched", got, err)
	}
}
