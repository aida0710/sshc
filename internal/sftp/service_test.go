package sftp_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"sshc/internal/sftp"
	"sshc/internal/validate"
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

type fileInfoWithSize struct {
	fs.FileInfo
	size int64
}

func (info fileInfoWithSize) Size() int64 { return info.size }

type sizeReportingRemote struct {
	*fakeRemote
	size int64
}

func (remote *sizeReportingRemote) Lstat(candidate string) (fs.FileInfo, error) {
	info, err := remote.fakeRemote.Lstat(candidate)
	if err != nil {
		return nil, err
	}
	return fileInfoWithSize{FileInfo: info, size: remote.size}, nil
}

type fakeRemote struct {
	nodes        map[string]node
	workingDir   string
	closed       bool
	replacements [][2]string
	removals     []string
	replaceErr   error
	replaceHook  func()
	closeHook    func()
	removeErr    error
	removeHook   func(string)
	writeErr     error
	createHook   func()
	openHook     func(string)
	lstatHook    func(string) error
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

func (r *fakeRemote) Close() error {
	if r.closeHook != nil {
		r.closeHook()
	}
	r.closed = true
	return nil
}

func (r *fakeRemote) Getwd(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if r.workingDir == "" {
		return "/", nil
	}
	return r.workingDir, nil
}

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
	if r.lstatHook != nil {
		if err := r.lstatHook(candidate); err != nil {
			return nil, err
		}
	}
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
	if r.openHook != nil {
		r.openHook(candidate)
	}
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

func (r *fakeRemote) OpenFile(candidate string, flags int) (sftp.WriteSeekCloser, error) {
	if flags != os.O_WRONLY {
		return nil, fs.ErrInvalid
	}
	info, ok := r.nodes[candidate]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return &fakeSeekWriter{remote: r, path: candidate, contents: append([]byte(nil), info.content...)}, nil
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
	if r.replaceHook != nil {
		r.replaceHook()
	}
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
	if r.removeHook != nil {
		r.removeHook(candidate)
	}
	if r.removeErr != nil {
		return r.removeErr
	}
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

type fakeSeekWriter struct {
	remote   *fakeRemote
	path     string
	contents []byte
	offset   int64
	closed   bool
}

func (w *fakeSeekWriter) Seek(offset int64, whence int) (int64, error) {
	if whence != io.SeekStart || offset < 0 {
		return 0, fs.ErrInvalid
	}
	w.offset = offset
	return offset, nil
}

func (w *fakeSeekWriter) Write(contents []byte) (int, error) {
	end := w.offset + int64(len(contents))
	if end > int64(len(w.contents)) {
		w.contents = append(w.contents, make([]byte, end-int64(len(w.contents)))...)
	}
	copy(w.contents[w.offset:end], contents)
	w.offset = end
	return len(contents), nil
}

func (w *fakeSeekWriter) Truncate(size int64) error {
	if size < 0 {
		return fs.ErrInvalid
	}
	if size < int64(len(w.contents)) {
		w.contents = w.contents[:size]
	} else if size > int64(len(w.contents)) {
		w.contents = append(w.contents, make([]byte, size-int64(len(w.contents)))...)
	}
	if w.offset > size {
		w.offset = size
	}
	return nil
}

func (w *fakeSeekWriter) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	info := w.remote.nodes[w.path]
	info.content = append([]byte(nil), w.contents...)
	info.modTime = w.remote.now()
	w.remote.nodes[w.path] = info
	return nil
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

func TestListDirectoryUsesRemoteWorkingDirectoryWhenPathIsOmitted(t *testing.T) {
	remote := remoteWith(map[string]node{
		"/home":            directory("home"),
		"/home/aida":       directory("aida"),
		"/home/aida/notes": file("notes", "hello", 0o600),
	})
	remote.workingDir = "/home/aida/./"

	listing, err := serviceFor(remote).ListDirectory(t.Context(), "edge", "")
	if err != nil {
		t.Fatalf("ListDirectory() = %v", err)
	}
	if listing.Path != "/home/aida" {
		t.Fatalf("path = %q, want /home/aida", listing.Path)
	}
	if len(listing.Entries) != 1 || listing.Entries[0].Path != "/home/aida/notes" {
		t.Fatalf("entries = %#v", listing.Entries)
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

func TestReadPreviewNamesTheTypeFromTheBytesAndRefusesEverythingElse(t *testing.T) {
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte("d"), 32)...)

	t.Run("png named as text", func(t *testing.T) {
		// 拡張子は中身について何も言わない。中身が PNG なら PNG と名乗る。
		remote := remoteWith(map[string]node{"/photo.txt": {name: "photo.txt", content: png, mode: 0o644, modTime: testTime}})
		preview, err := serviceFor(remote).ReadPreview(context.Background(), "edge", "/photo.txt")
		if err != nil {
			t.Fatalf("ReadPreview() = %v", err)
		}
		if preview.ContentType != "image/png" || !bytes.Equal(preview.Contents, png) {
			t.Fatalf("preview = %q %d bytes", preview.ContentType, len(preview.Contents))
		}
		if preview.Entry.Path != "/photo.txt" || !strings.HasPrefix(preview.Revision, "content-sha256:") {
			t.Fatalf("entry = %#v revision = %q", preview.Entry, preview.Revision)
		}
	})

	tests := []struct {
		name     string
		contents []byte
		mode     fs.FileMode
		want     error
	}{
		// 名前が png でも、中身が png でなければ preview にはしない。
		{name: "html named as png", contents: []byte("<html><script>alert(1)</script>"), mode: 0o644, want: sftp.ErrPreviewType},
		{name: "svg", contents: []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), mode: 0o644, want: sftp.ErrPreviewType},
		{name: "utf8 text", contents: []byte("一行目\n"), mode: 0o644, want: sftp.ErrPreviewType},
		// PDF は <iframe> でしか描けず、CSP を緩めることになるので preview しない。
		{name: "pdf", contents: []byte("%PDF-1.7\ntrailer"), mode: 0o644, want: sftp.ErrPreviewType},
		{name: "directory", contents: nil, mode: fs.ModeDir | 0o755, want: sftp.ErrNotRegularFile},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			remote := remoteWith(map[string]node{"/candidate.png": {
				name: "candidate.png", content: test.contents, mode: test.mode, modTime: testTime,
			}})
			if _, err := serviceFor(remote).ReadPreview(context.Background(), "edge", "/candidate.png"); !errors.Is(err, test.want) {
				t.Fatalf("ReadPreview() = %v, want %v", err, test.want)
			}
		})
	}

	t.Run("too large", func(t *testing.T) {
		oversized := append(append([]byte{}, png...), bytes.Repeat([]byte("d"), sftp.MaxPreviewFileBytes)...)
		remote := remoteWith(map[string]node{"/huge.png": {name: "huge.png", content: oversized, mode: 0o644, modTime: testTime}})
		if _, err := serviceFor(remote).ReadPreview(context.Background(), "edge", "/huge.png"); !errors.Is(err, sftp.ErrPreviewTooLarge) {
			t.Fatalf("ReadPreview() = %v, want %v", err, sftp.ErrPreviewTooLarge)
		}
	})
}

func TestSearchMatchesNamesBelowARootWithoutFollowingSymlinks(t *testing.T) {
	remote := remoteWith(map[string]node{
		"/srv":                    directory("srv"),
		"/srv/app":                directory("app"),
		"/srv/app/report.log":     file("report.log", "one", 0o644),
		"/srv/app/notes.txt":      file("notes.txt", "two", 0o644),
		"/srv/app/logs":           directory("logs"),
		"/srv/app/logs/other.log": file("other.log", "three", 0o644),
		"/srv/elsewhere.log":      file("elsewhere.log", "four", 0o644),
		"/outside.log":            file("outside.log", "five", 0o644),
	})
	service := serviceFor(remote)

	found, err := service.Search(context.Background(), "edge", "/srv", "LOG")
	if err != nil {
		t.Fatalf("Search() = %v", err)
	}
	if found.Path != "/srv" || found.Query != "LOG" || found.Truncated {
		t.Fatalf("result = %+v", found)
	}
	paths := make([]string, 0, len(found.Entries))
	for _, entry := range found.Entries {
		paths = append(paths, entry.Path)
	}
	sort.Strings(paths)
	// 大文字小文字は問わず、rootの外へは出ず、ディレクトリ自身も一致する。
	want := []string{"/srv/app/logs", "/srv/app/logs/other.log", "/srv/app/report.log", "/srv/elsewhere.log"}
	if !slices.Equal(paths, want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}

	for _, query := range []string{"", "   ", strings.Repeat("x", sftp.MaxSearchQueryBytes+1)} {
		if _, err := service.Search(context.Background(), "edge", "/srv", query); !errors.Is(err, sftp.ErrInvalidQuery) {
			t.Fatalf("Search(%q) = %v, want %v", query, err, sftp.ErrInvalidQuery)
		}
	}
	if _, err := service.Search(context.Background(), "edge", "relative", "log"); !errors.Is(err, sftp.ErrInvalidPath) {
		t.Fatalf("Search(relative) = %v, want %v", err, sftp.ErrInvalidPath)
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

func TestDownloadMkdirRenameAndDelete(t *testing.T) {
	remote := remoteWith(map[string]node{
		"/home":       directory("home"),
		"/home/a":     file("a", "contents", 0o600),
		"/home/taken": file("taken", "x", 0o600),
	})
	service := serviceFor(remote)
	var downloaded bytes.Buffer
	prepared, err := service.PrepareDownload(context.Background(), "edge", "/home/a")
	if err != nil {
		t.Fatalf("PrepareDownload() = %v", err)
	}
	defer prepared.Close()
	written, err := prepared.WriteFrom(context.Background(), 0, &downloaded)
	if err != nil {
		t.Fatalf("Download() = %v", err)
	}
	if downloaded.String() != "contents" || written != 8 {
		t.Fatalf("download = %q, written = %d", downloaded.String(), written)
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

func TestPrepareDownloadRejectsFilesLargerThan512GiBBeforeReading(t *testing.T) {
	base := remoteWith(map[string]node{
		"/oversized.bin": {name: "oversized.bin", mode: 0o600, modTime: testTime},
	})
	opened := false
	base.openHook = func(string) { opened = true }
	remote := &sizeReportingRemote{fakeRemote: base, size: (512 << 30) + 1}
	service := &sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) {
		return remote, nil
	}}

	if _, err := service.PrepareDownload(t.Context(), "edge", "/oversized.bin"); !errors.Is(err, sftp.ErrTransferTooLarge) {
		t.Fatalf("PrepareDownload(512 GiB + 1) = %v, want ErrTransferTooLarge", err)
	}
	if opened {
		t.Fatal("oversized file was opened before its size was rejected")
	}
}

func TestPrepareDownloadRevisionBindsTheExactServedContents(t *testing.T) {
	remote := remoteWith(map[string]node{
		"/report.bin": {name: "report.bin", mode: 0o600, content: []byte("old-data"), modTime: testTime},
	})
	service := serviceFor(remote)

	first, err := service.PrepareDownload(t.Context(), "edge", "/report.bin")
	if err != nil {
		t.Fatalf("prepare first download: %v", err)
	}
	defer first.Close()

	// SFTP v3 metadata cannot distinguish this replacement: the size, mode and
	// second-resolution modification time are intentionally identical.
	remote.nodes["/report.bin"] = node{
		name: "report.bin", mode: 0o600, content: []byte("new-data"), modTime: testTime,
	}
	second, err := service.PrepareDownload(t.Context(), "edge", "/report.bin")
	if err != nil {
		t.Fatalf("prepare replacement download: %v", err)
	}
	defer second.Close()

	if first.Revision == second.Revision {
		t.Fatalf("content revisions matched for distinct bytes: %q", first.Revision)
	}
	var firstContents, secondContents bytes.Buffer
	if _, err := first.WriteFrom(t.Context(), 0, &firstContents); err != nil {
		t.Fatalf("read first prepared download: %v", err)
	}
	if _, err := second.WriteFrom(t.Context(), 0, &secondContents); err != nil {
		t.Fatalf("read replacement prepared download: %v", err)
	}
	if firstContents.String() != "old-data" || secondContents.String() != "new-data" {
		t.Fatalf("prepared contents = %q, %q", firstContents.String(), secondContents.String())
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

func TestDownloadArchiveSkipsUnpublishedUploadParts(t *testing.T) {
	remote := remoteWith(map[string]node{
		"/project":         directory("project"),
		"/project/visible": file("visible", "ok", 0o600),
		"/project/.secret.sshc-upload-transfer_hidden1.part": file(".secret.sshc-upload-transfer_hidden1.part", "partial-secret", 0o600),
	})
	var contents bytes.Buffer
	if _, err := serviceFor(remote).DownloadArchive(t.Context(), "edge", "/project", &contents); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(contents.Bytes()), int64(contents.Len()))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range reader.File {
		if strings.Contains(entry.Name, "sshc-upload") {
			t.Fatalf("partial upload leaked as %q", entry.Name)
		}
	}
}

func TestMkdirTreatsAnExistingDirectoryAsSuccess(t *testing.T) {
	remote := remoteWith(map[string]node{"/project": directory("project")})
	entry, err := serviceFor(remote).Mkdir(t.Context(), "edge", "/project")
	if err != nil || entry.Type != sftp.EntryDirectory {
		t.Fatalf("existing directory = %+v, %v", entry, err)
	}
	remote.nodes["/file"] = file("file", "x", 0o600)
	if _, err := serviceFor(remote).Mkdir(t.Context(), "edge", "/file"); !errors.Is(err, sftp.ErrAlreadyExists) {
		t.Fatalf("existing file = %v", err)
	}
}

func TestDownloadArchiveRejectsARegularFileThatGrowsAfterListing(t *testing.T) {
	remote := remoteWith(map[string]node{
		"/project":   directory("project"),
		"/project/a": file("a", "a", 0o600),
	})
	remote.openHook = func(candidate string) {
		if candidate != "/project/a" {
			return
		}
		changed := remote.nodes[candidate]
		changed.content = []byte("expanded")
		remote.nodes[candidate] = changed
		remote.openHook = nil
	}
	var destination bytes.Buffer
	if _, err := serviceFor(remote).DownloadArchive(t.Context(), "edge", "/project", &destination); !errors.Is(err, sftp.ErrConflict) {
		t.Fatalf("growing archive entry = %v", err)
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
		{name: "hostile alias", run: func() error { _, err := service.List(context.Background(), "$(touch-pwned)", "/"); return err }, want: validate.ErrUnsafeAlias},
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

func TestInternalTransferPathsAreHiddenAndRejectedByPublicOperations(t *testing.T) {
	const uploadPart = "/.report.sshc-upload-transfer_12345678.part"
	const editorTemporary = "/.report.sshc-0123456789abcdef01234567.tmp"
	remote := remoteWith(map[string]node{
		"/report":       file("report", "published", 0o600),
		uploadPart:      file(path.Base(uploadPart), "partial", 0o600),
		editorTemporary: file(path.Base(editorTemporary), "staged", 0o600),
	})
	service := serviceFor(remote)
	entries, err := service.List(t.Context(), "edge", "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Path != "/report" {
		t.Fatalf("public entries = %#v, want only /report", entries)
	}
	for _, internal := range []string{uploadPart, editorTemporary} {
		if _, err := service.Stat(t.Context(), "edge", internal); !errors.Is(err, sftp.ErrInvalidPath) {
			t.Errorf("Stat(%q) = %v, want ErrInvalidPath", internal, err)
		}
		if err := service.Delete(t.Context(), "edge", internal); !errors.Is(err, sftp.ErrInvalidPath) {
			t.Errorf("Delete(%q) = %v, want ErrInvalidPath", internal, err)
		}
		if _, err := service.Rename(t.Context(), "edge", internal, "/published"); !errors.Is(err, sftp.ErrInvalidPath) {
			t.Errorf("Rename(%q) = %v, want ErrInvalidPath", internal, err)
		}
	}
}

type blockingReadRemote struct {
	*fakeRemote
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (remote *blockingReadRemote) Open(string) (io.ReadCloser, error) {
	remote.once.Do(func() { close(remote.entered) })
	return blockingReadCloser{release: remote.release}, nil
}

func (remote *blockingReadRemote) Close() error {
	select {
	case <-remote.release:
	default:
		close(remote.release)
	}
	return nil
}

type blockingReadCloser struct{ release <-chan struct{} }

func (reader blockingReadCloser) Read([]byte) (int, error) {
	<-reader.release
	return 0, fs.ErrClosed
}

func (blockingReadCloser) Close() error { return nil }

func TestRequestCancellationClosesABlockedRemoteRead(t *testing.T) {
	remote := &blockingReadRemote{
		fakeRemote: remoteWith(map[string]node{"/large.bin": file("large.bin", "x", 0o600)}),
		entered:    make(chan struct{}),
		release:    make(chan struct{}),
	}
	service := sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		_, err := service.PrepareDownload(ctx, "edge", "/large.bin")
		done <- err
	}()
	<-remote.entered
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("PrepareDownload cancellation = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancel did not close the blocked SFTP transport")
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
