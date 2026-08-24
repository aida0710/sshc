package sftp_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
	"testing"
	"time"

	"sshc/internal/sftp"
)

var testTime = time.Date(2026, 8, 24, 7, 0, 0, 0, time.UTC)

type node struct {
	name    string
	mode    fs.FileMode
	content []byte
	modTime time.Time
}

func (n node) Name() string       { return n.name }
func (n node) Size() int64        { return int64(len(n.content)) }
func (n node) Mode() fs.FileMode  { return n.mode }
func (n node) ModTime() time.Time { return n.modTime }
func (n node) IsDir() bool        { return n.mode.IsDir() }
func (n node) Sys() any           { return nil }

type fakeRemote struct {
	nodes        map[string]node
	closed       bool
	replacements [][2]string
	removals     []string
	replaceErr   error
	writeErr     error
	createHook   func()
	tick         int
}

func remoteWith(entries map[string]node) *fakeRemote {
	if entries == nil {
		entries = make(map[string]node)
	}
	if _, ok := entries["/"]; !ok {
		entries["/"] = node{name: "/", mode: fs.ModeDir | 0o755, modTime: testTime}
	}
	return &fakeRemote{nodes: entries}
}

func (r *fakeRemote) Close() error { r.closed = true; return nil }

func (r *fakeRemote) ReadDir(ctx context.Context, directory string) ([]fs.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if info, ok := r.nodes[directory]; !ok {
		return nil, fs.ErrNotExist
	} else if !info.IsDir() {
		return nil, fs.ErrInvalid
	}
	var infos []fs.FileInfo
	for candidate, info := range r.nodes {
		if candidate != directory && path.Dir(candidate) == directory {
			infos = append(infos, info)
		}
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Name() < infos[j].Name() })
	return infos, nil
}

func (r *fakeRemote) Lstat(candidate string) (fs.FileInfo, error) {
	info, ok := r.nodes[candidate]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return info, nil
}

func (r *fakeRemote) ReadLink(candidate string) (string, error) {
	info, ok := r.nodes[candidate]
	if !ok {
		return "", fs.ErrNotExist
	}
	if info.mode&fs.ModeSymlink == 0 {
		return "", fs.ErrInvalid
	}
	return string(info.content), nil
}

func (r *fakeRemote) Open(candidate string) (io.ReadCloser, error) {
	info, ok := r.nodes[candidate]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(info.content)), nil
}

func (r *fakeRemote) Create(candidate string) (io.WriteCloser, error) {
	r.nodes[candidate] = node{name: path.Base(candidate), mode: 0o666, modTime: testTime}
	if r.createHook != nil {
		r.createHook()
	}
	return &fakeWriter{remote: r, path: candidate}, nil
}

func (r *fakeRemote) Mkdir(candidate string) error {
	if _, ok := r.nodes[candidate]; ok {
		return fs.ErrExist
	}
	r.nodes[candidate] = node{name: path.Base(candidate), mode: fs.ModeDir | 0o755, modTime: r.now()}
	return nil
}

func (r *fakeRemote) Chmod(candidate string, mode fs.FileMode) error {
	info, ok := r.nodes[candidate]
	if !ok {
		return fs.ErrNotExist
	}
	info.mode = mode
	r.nodes[candidate] = info
	return nil
}

func (r *fakeRemote) Replace(from, to string) error {
	r.replacements = append(r.replacements, [2]string{from, to})
	if r.replaceErr != nil {
		return r.replaceErr
	}
	return r.move(from, to)
}

func (r *fakeRemote) Rename(from, to string) error { return r.move(from, to) }

func (r *fakeRemote) move(from, to string) error {
	info, ok := r.nodes[from]
	if !ok {
		return fs.ErrNotExist
	}
	delete(r.nodes, from)
	info.name = path.Base(to)
	info.modTime = r.now()
	r.nodes[to] = info
	return nil
}

func (r *fakeRemote) Remove(candidate string) error {
	r.removals = append(r.removals, candidate)
	if _, ok := r.nodes[candidate]; !ok {
		return fs.ErrNotExist
	}
	delete(r.nodes, candidate)
	return nil
}

func (r *fakeRemote) RemoveDirectory(candidate string) error {
	for existing := range r.nodes {
		if existing != candidate && path.Dir(existing) == candidate {
			return errors.New("directory not empty")
		}
	}
	return r.Remove(candidate)
}

func (r *fakeRemote) now() time.Time {
	r.tick++
	return testTime.Add(time.Duration(r.tick) * time.Second)
}

type fakeWriter struct {
	remote *fakeRemote
	path   string
	buffer bytes.Buffer
	closed bool
}

func (w *fakeWriter) Write(contents []byte) (int, error) {
	if w.remote.writeErr != nil {
		return 0, w.remote.writeErr
	}
	return w.buffer.Write(contents)
}

func (w *fakeWriter) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	info := w.remote.nodes[w.path]
	info.content = append([]byte(nil), w.buffer.Bytes()...)
	info.modTime = w.remote.now()
	w.remote.nodes[w.path] = info
	return nil
}

func serviceFor(remote *fakeRemote) sftp.Service {
	return sftp.Service{
		Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil },
		TemporaryPath: func(target string) (string, error) {
			return path.Join(path.Dir(target), ".upload.sshc.tmp"), nil
		},
	}
}

func file(name, contents string, mode fs.FileMode) node {
	return node{name: name, content: []byte(contents), mode: mode, modTime: testTime}
}

func directory(name string) node {
	return node{name: name, mode: fs.ModeDir | 0o755, modTime: testTime}
}

func TestListAndStatExposeStableMetadata(t *testing.T) {
	remote := remoteWith(map[string]node{
		"/home":          directory("home"),
		"/home/z.txt":    file("z.txt", "z", 0o640),
		"/home/projects": directory("projects"),
		"/home/a.txt":    file("a.txt", "alpha", 0o600),
	})
	service := serviceFor(remote)

	entries, err := service.List(context.Background(), "edge", "/home/./")
	if err != nil {
		t.Fatalf("List() = %v", err)
	}
	got := []string{entries[0].Name, entries[1].Name, entries[2].Name}
	want := []string{"projects", "a.txt", "z.txt"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("names = %v, want %v", got, want)
	}
	if entries[0].Type != sftp.EntryDirectory || entries[1].Type != sftp.EntryFile {
		t.Fatalf("entry types = %#v", entries)
	}
	if !strings.HasPrefix(entries[1].Revision, "meta-sha256:") {
		t.Fatalf("revision = %q", entries[1].Revision)
	}

	entry, err := service.Stat(context.Background(), "edge", "/home/a.txt")
	if err != nil {
		t.Fatalf("Stat() = %v", err)
	}
	if entry.Path != "/home/a.txt" || entry.Mode.Perm() != 0o600 || entry.Size != 5 {
		t.Fatalf("entry = %#v", entry)
	}
	if !remote.closed {
		t.Fatal("remote was not closed")
	}
}

func TestReadTextAcceptsUTF8AndRejectsBinaryOrLargeFiles(t *testing.T) {
	t.Run("utf8", func(t *testing.T) {
		remote := remoteWith(map[string]node{"/メモ.txt": file("メモ.txt", "一行目\n", 0o640)})
		text, err := serviceFor(remote).ReadText(context.Background(), "edge", "/メモ.txt")
		if err != nil {
			t.Fatalf("ReadText() = %v", err)
		}
		if text.Contents != "一行目\n" || !strings.HasPrefix(text.Revision, "content-sha256:") {
			t.Fatalf("text = %#v", text)
		}
	})

	tests := []struct {
		name     string
		contents []byte
		want     error
	}{
		{name: "nul", contents: []byte("before\x00after"), want: sftp.ErrNotUTF8},
		{name: "invalid utf8", contents: []byte{0xff, 0xfe}, want: sftp.ErrNotUTF8},
		{name: "too large", contents: bytes.Repeat([]byte("x"), sftp.MaxEditableFileBytes+1), want: sftp.ErrTextTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			remote := remoteWith(map[string]node{"/file": {
				name: "file", content: test.contents, mode: 0o600, modTime: testTime,
			}})
			_, err := serviceFor(remote).ReadText(context.Background(), "edge", "/file")
			if !errors.Is(err, test.want) {
				t.Fatalf("ReadText() = %v, want %v", err, test.want)
			}
		})
	}
}

func TestSaveTextRequiresCurrentContentRevisionAndAtomicallyReplaces(t *testing.T) {
	remote := remoteWith(map[string]node{"/config": file("config", "before\n", 0o640)})
	service := serviceFor(remote)
	opened, err := service.ReadText(context.Background(), "edge", "/config")
	if err != nil {
		t.Fatalf("ReadText() = %v", err)
	}

	remote.nodes["/config"] = file("config", "changed elsewhere\n", 0o640)
	remote.nodes["/config"] = withTime(remote.nodes["/config"], testTime.Add(time.Minute))
	if _, err := service.SaveText(context.Background(), "edge", "/config", "mine\n", opened.Revision); !errors.Is(err, sftp.ErrConflict) {
		t.Fatalf("SaveText(stale) = %v, want ErrConflict", err)
	}
	if got := string(remote.nodes["/config"].content); got != "changed elsewhere\n" {
		t.Fatalf("stale save changed contents to %q", got)
	}

	current, err := service.ReadText(context.Background(), "edge", "/config")
	if err != nil {
		t.Fatalf("ReadText(current) = %v", err)
	}
	saved, err := service.SaveText(context.Background(), "edge", "/config", "saved\r\n", current.Revision)
	if err != nil {
		t.Fatalf("SaveText() = %v", err)
	}
	if saved.Contents != "saved\r\n" || remote.nodes["/config"].mode.Perm() != 0o640 {
		t.Fatalf("saved = %#v, node = %#v", saved, remote.nodes["/config"])
	}
	if len(remote.replacements) != 1 || remote.replacements[0] != [2]string{"/.upload.sshc.tmp", "/config"} {
		t.Fatalf("replacements = %v", remote.replacements)
	}
	if _, ok := remote.nodes["/.upload.sshc.tmp"]; ok {
		t.Fatal("temporary file remained")
	}
}

func TestUploadProtectsExistingFilesAndCleansFailedTemporaryFiles(t *testing.T) {
	remote := remoteWith(map[string]node{"/data.bin": file("data.bin", "old", 0o644)})
	service := serviceFor(remote)
	entry, err := service.Stat(context.Background(), "edge", "/data.bin")
	if err != nil {
		t.Fatalf("Stat() = %v", err)
	}
	if _, err := service.Upload(context.Background(), "edge", "/data.bin", strings.NewReader("new"), sftp.UploadOptions{}); !errors.Is(err, sftp.ErrAlreadyExists) {
		t.Fatalf("Upload(existing) = %v, want ErrAlreadyExists", err)
	}
	if _, err := service.Upload(context.Background(), "edge", "/data.bin", strings.NewReader("new"), sftp.UploadOptions{
		Overwrite: true, ExpectedRevision: "meta-sha256:stale",
	}); !errors.Is(err, sftp.ErrConflict) {
		t.Fatalf("Upload(stale) = %v, want ErrConflict", err)
	}

	transfer, err := service.Upload(context.Background(), "edge", "/data.bin", strings.NewReader("new"), sftp.UploadOptions{
		Overwrite: true, ExpectedRevision: entry.Revision,
	})
	if err != nil {
		t.Fatalf("Upload() = %v", err)
	}
	if transfer.Bytes != 3 || string(remote.nodes["/data.bin"].content) != "new" || remote.nodes["/data.bin"].mode.Perm() != 0o644 {
		t.Fatalf("transfer = %#v, node = %#v", transfer, remote.nodes["/data.bin"])
	}

	if _, err := service.Upload(context.Background(), "edge", "/large", strings.NewReader("12345"), sftp.UploadOptions{MaxBytes: 4}); !errors.Is(err, sftp.ErrTransferTooLarge) {
		t.Fatalf("Upload(large) = %v, want ErrTransferTooLarge", err)
	}
	if _, ok := remote.nodes["/.upload.sshc.tmp"]; ok {
		t.Fatal("failed upload left a temporary file")
	}
}

func TestDownloadMkdirRenameAndDelete(t *testing.T) {
	remote := remoteWith(map[string]node{
		"/home":       directory("home"),
		"/home/a":     file("a", "contents", 0o600),
		"/home/taken": file("taken", "x", 0o600),
	})
	service := serviceFor(remote)
	var downloaded bytes.Buffer
	transfer, err := service.Download(context.Background(), "edge", "/home/a", &downloaded)
	if err != nil {
		t.Fatalf("Download() = %v", err)
	}
	if downloaded.String() != "contents" || transfer.Bytes != 8 {
		t.Fatalf("download = %q, transfer = %#v", downloaded.String(), transfer)
	}
	if _, err := service.Mkdir(context.Background(), "edge", "/home/new"); err != nil {
		t.Fatalf("Mkdir() = %v", err)
	}
	if _, err := service.Rename(context.Background(), "edge", "/home/a", "/home/taken"); !errors.Is(err, sftp.ErrAlreadyExists) {
		t.Fatalf("Rename(existing) = %v, want ErrAlreadyExists", err)
	}
	if _, err := service.Rename(context.Background(), "edge", "/home/a", "/home/b"); err != nil {
		t.Fatalf("Rename() = %v", err)
	}
	if err := service.Delete(context.Background(), "edge", "/home/b"); err != nil {
		t.Fatalf("Delete(file) = %v", err)
	}
	if err := service.Delete(context.Background(), "edge", "/home/new"); err != nil {
		t.Fatalf("Delete(directory) = %v", err)
	}
	if _, ok := remote.nodes["/home/b"]; ok {
		t.Fatal("renamed file remained after delete")
	}
}

func TestDownloadArchiveAndChmod(t *testing.T) {
	remote := remoteWith(map[string]node{
		"/home":               directory("home"),
		"/home/project":       directory("project"),
		"/home/project/a":     file("a", "alpha", 0o640),
		"/home/project/link":  {name: "link", content: []byte("../escape"), mode: fs.ModeSymlink | 0o777, modTime: testTime},
		"/home/project/sub":   directory("sub"),
		"/home/project/sub/b": file("b", "beta", 0o600),
	})
	service := serviceFor(remote)
	var contents bytes.Buffer
	transfer, err := service.DownloadArchive(context.Background(), "edge", "/home/project", &contents)
	if err != nil {
		t.Fatalf("DownloadArchive() = %v", err)
	}
	if transfer.Bytes != 18 {
		t.Fatalf("archive bytes = %d, want 18", transfer.Bytes)
	}
	reader, err := zip.NewReader(bytes.NewReader(contents.Bytes()), int64(contents.Len()))
	if err != nil {
		t.Fatalf("zip.NewReader() = %v", err)
	}
	var names []string
	for _, entry := range reader.File {
		names = append(names, entry.Name)
	}
	if got := strings.Join(names, ","); got != "project/,project/a,project/link,project/sub/,project/sub/b" {
		t.Fatalf("archive entries = %q", got)
	}
	link := reader.File[2]
	if link.Mode()&fs.ModeSymlink != 0 || !link.Mode().IsRegular() {
		t.Fatalf("archived link mode = %v", link.Mode())
	}
	opened, err := link.Open()
	if err != nil {
		t.Fatal(err)
	}
	target, err := io.ReadAll(opened)
	_ = opened.Close()
	if err != nil || string(target) != "../escape" {
		t.Fatalf("archived link = %q, %v", target, err)
	}

	before, err := service.Stat(context.Background(), "edge", "/home/project/a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Chmod(context.Background(), "edge", "/home/project/a", 0o600, "stale"); !errors.Is(err, sftp.ErrConflict) {
		t.Fatalf("Chmod(stale) = %v, want conflict", err)
	}
	changed, err := service.Chmod(context.Background(), "edge", "/home/project/a", 0o600, before.Revision)
	if err != nil {
		t.Fatalf("Chmod() = %v", err)
	}
	if changed.Mode.Perm() != 0o600 {
		t.Fatalf("mode = %o", changed.Mode.Perm())
	}
}

func TestUnsafeInputsFailBeforeOpeningAConnection(t *testing.T) {
	opened := 0
	service := sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) {
		opened++
		return remoteWith(nil), nil
	}}
	tests := []struct {
		name string
		run  func() error
		want error
	}{
		{name: "relative list", run: func() error { _, err := service.List(context.Background(), "edge", "tmp"); return err }, want: sftp.ErrInvalidPath},
		{name: "root delete", run: func() error { return service.Delete(context.Background(), "edge", "/") }, want: sftp.ErrRootOperation},
		{name: "root mkdir", run: func() error { _, err := service.Mkdir(context.Background(), "edge", "/"); return err }, want: sftp.ErrRootOperation},
		{name: "empty alias", run: func() error { _, err := service.Stat(context.Background(), " ", "/"); return err }, want: sftp.ErrInvalidAlias},
		{name: "missing revision", run: func() error { _, err := service.SaveText(context.Background(), "edge", "/file", "x", ""); return err }, want: sftp.ErrRevisionRequired},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
	if opened != 0 {
		t.Fatalf("opened = %d, want 0", opened)
	}
}

func TestReplaceFailureKeepsDestinationAndRemovesTemporaryFile(t *testing.T) {
	remote := remoteWith(map[string]node{"/config": file("config", "before", 0o600)})
	service := serviceFor(remote)
	opened, err := service.ReadText(context.Background(), "edge", "/config")
	if err != nil {
		t.Fatalf("ReadText() = %v", err)
	}
	remote.replaceErr = errors.New("rename refused")
	if _, err := service.SaveText(context.Background(), "edge", "/config", "after", opened.Revision); err == nil || err.Error() != "rename refused" {
		t.Fatalf("SaveText() = %v", err)
	}
	if got := string(remote.nodes["/config"].content); got != "before" {
		t.Fatalf("destination = %q, want before", got)
	}
	if _, ok := remote.nodes["/.upload.sshc.tmp"]; ok {
		t.Fatal("temporary file remained")
	}
}

func TestSaveTextRechecksRevisionAfterStagingTheReplacement(t *testing.T) {
	remote := remoteWith(map[string]node{"/config": file("config", "before", 0o600)})
	service := serviceFor(remote)
	opened, err := service.ReadText(context.Background(), "edge", "/config")
	if err != nil {
		t.Fatalf("ReadText() = %v", err)
	}
	remote.createHook = func() {
		changed := file("config", "changed while staging", 0o600)
		changed.modTime = testTime.Add(time.Minute)
		remote.nodes["/config"] = changed
		remote.createHook = nil
	}
	if _, err := service.SaveText(context.Background(), "edge", "/config", "mine", opened.Revision); !errors.Is(err, sftp.ErrConflict) {
		t.Fatalf("SaveText() = %v, want ErrConflict", err)
	}
	if got := string(remote.nodes["/config"].content); got != "changed while staging" {
		t.Fatalf("destination = %q", got)
	}
	if _, ok := remote.nodes["/.upload.sshc.tmp"]; ok {
		t.Fatal("temporary file remained")
	}
}

func withTime(info node, modTime time.Time) node {
	info.modTime = modTime
	return info
}
