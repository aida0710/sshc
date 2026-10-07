package sftp_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sshc/internal/sftp"
)

// Model FSTAT independently of LSTAT, so replacing a path does not replace
// the identity/metadata of an already opened handle.
type contentReadRemote struct {
	*fakeRemote
	readBytes int64
	opened    []string
	sizes     map[string]int64
	onRead    func(string)
}

type contentReadFile struct {
	*bytes.Reader
	metadata  fs.FileInfo
	remote    *contentReadRemote
	candidate string
}

func (file *contentReadFile) Close() error               { return nil }
func (file *contentReadFile) Stat() (fs.FileInfo, error) { return file.metadata, nil }
func (file *contentReadFile) Read(buffer []byte) (int, error) {
	read, err := file.Reader.Read(buffer)
	file.remote.readBytes += int64(read)
	if file.remote.onRead != nil {
		file.remote.onRead(file.candidate)
	}
	return read, err
}

func contentFixtureFile(name, contents string) node { return file(name, contents, 0o644) }

func contentRemote(entries map[string]node) *contentReadRemote {
	return &contentReadRemote{fakeRemote: remoteWith(entries), sizes: make(map[string]int64)}
}

func (remote *contentReadRemote) Lstat(candidate string) (fs.FileInfo, error) {
	info, err := remote.fakeRemote.Lstat(candidate)
	if size, exists := remote.sizes[candidate]; exists && err == nil {
		return fileInfoWithSize{FileInfo: info, size: size}, nil
	}
	return info, err
}

func (remote *contentReadRemote) ReadDir(ctx context.Context, directory string) ([]fs.FileInfo, error) {
	children, err := remote.fakeRemote.ReadDir(ctx, directory)
	for index, child := range children {
		if size, exists := remote.sizes[path.Join(directory, child.Name())]; exists {
			children[index] = fileInfoWithSize{FileInfo: child, size: size}
		}
	}
	return children, err
}

func (remote *contentReadRemote) Open(candidate string) (io.ReadCloser, error) {
	if remote.openHook != nil {
		remote.openHook(candidate)
	}
	info, err := remote.Lstat(candidate)
	if err != nil {
		return nil, err
	}
	remote.opened = append(remote.opened, candidate)
	return &contentReadFile{Reader: bytes.NewReader(remote.nodes[candidate].content), metadata: info, remote: remote, candidate: candidate}, nil
}

func contentService(remote *contentReadRemote) sftp.Service {
	return sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }}
}

func TestContentSearchReturnsLiteralMatchesAcrossFilesAtTheirLineNumbers(t *testing.T) {
	remote := contentRemote(map[string]node{
		"/work": directory("work"), "/work/sub": directory("sub"),
		"/work/a.txt":     contentFixtureFile("a.txt", "first\nneedle.* 一致\nNEEDLE.*\n"),
		"/work/sub/b.txt": contentFixtureFile("b.txt", "needle.*\nsecond\nneedle.* again\n"),
	})
	found, err := contentService(remote).Search(t.Context(), sftp.SearchOptions{Alias: "edge", Path: "/work", Query: "needle.*", Mode: sftp.SearchContent})
	if err != nil {
		t.Fatal(err)
	}
	if len(found.Matches) != 3 || found.Truncated || found.Matches[0].Entry.Path != "/work/a.txt" || found.Matches[0].Line != 2 || found.Matches[2].Line != 3 {
		t.Fatalf("content matches = %+v", found)
	}
	if found.BytesRead != remote.readBytes || !strings.Contains(found.Matches[0].Snippet, "一致") {
		t.Fatalf("read/snippet = %+v", found)
	}
}

func TestContentSearchUsesTheEditorsLineNumbersForCRLFAndCREndings(t *testing.T) {
	remote := contentRemote(map[string]node{"/work": directory("work"), "/work/a": contentFixtureFile("a", "first\rneedle\r\nthird\nneedle")})
	found, err := contentService(remote).Search(t.Context(), sftp.SearchOptions{Alias: "edge", Path: "/work", Query: "needle", Mode: sftp.SearchContent})
	if err != nil || len(found.Matches) != 2 || found.Matches[0].Line != 2 || found.Matches[1].Line != 4 {
		t.Fatalf("line endings = %+v, %v", found, err)
	}
}

func TestOpeningAContentSearchMatchRefusesChangedMetadataAndNewLinks(t *testing.T) {
	remote := contentRemote(map[string]node{"/work": directory("work"), "/work/a": contentFixtureFile("a", "needle"), "/outside": contentFixtureFile("outside", "needle")})
	service := contentService(remote)
	found, err := service.Search(t.Context(), sftp.SearchOptions{Alias: "edge", Path: "/work", Query: "needle", Mode: sftp.SearchContent})
	if err != nil {
		t.Fatal(err)
	}
	options := sftp.TextReadOptions{Alias: "edge", Path: "/work/a", ExpectedRevision: found.Matches[0].Entry.Revision}
	opened, err := service.ReadTextWithRevision(t.Context(), options)
	if err != nil || opened.Contents != "needle" {
		t.Fatalf("open match = %+v, %v", opened, err)
	}
	reads := remote.readBytes
	remote.nodes["/work/a"] = symlink("a", "/outside")
	_, err = service.ReadTextWithRevision(t.Context(), options)
	if !errors.Is(err, sftp.ErrConflict) || remote.readBytes != reads {
		t.Fatalf("changed link = %v, read %d after %d", err, remote.readBytes, reads)
	}
	delete(remote.nodes, "/work/a")
	_, err = service.ReadTextWithRevision(t.Context(), options)
	if !errors.Is(err, sftp.ErrConflict) {
		t.Fatalf("removed search match = %v", err)
	}
}

func TestContentSearchSkipsLinksBinaryOversizedAndUnreadableFilesWithReasons(t *testing.T) {
	remote := contentRemote(map[string]node{
		"/work": directory("work"), "/work/good.txt": contentFixtureFile("good.txt", "needle\n"),
		"/work/link": symlink("link", "/outside"), "/work/binary": contentFixtureFile("binary", "needle\x00"),
		"/work/invalid": contentFixtureFile("invalid", "needle\xff"), "/work/large": contentFixtureFile("large", "needle"),
		"/work/denied": directory("denied"), "/work/denied/file": contentFixtureFile("file", "needle"),
		"/outside": contentFixtureFile("outside", "needle"),
	})
	remote.sizes["/work/large"] = sftp.MaxEditableFileBytes + 1
	remote.readDirHook = func(candidate string) error {
		if candidate == "/work/denied" {
			return fs.ErrPermission
		}
		return nil
	}
	found, err := contentService(remote).Search(t.Context(), sftp.SearchOptions{Alias: "edge", Path: "/work", Query: "needle", Mode: sftp.SearchContent})
	if err != nil {
		t.Fatal(err)
	}
	if !found.Truncated || len(found.Matches) != 1 {
		t.Fatalf("partial matches = %+v", found)
	}
	omissions := make(map[string]int)
	for _, omission := range found.Omissions {
		omissions[omission.Reason] = omission.Count
	}
	for reason, want := range map[string]int{"symlink": 1, "binary": 2, "file_size": 1, "unreadable": 1} {
		if omissions[reason] != want {
			t.Fatalf("%s = %d, want %d", reason, omissions[reason], want)
		}
	}
	for _, opened := range remote.opened {
		if opened == "/outside" || opened == "/work/link" || opened == "/work/large" {
			t.Fatalf("unsafe open: %s", opened)
		}
	}
}

func TestContentSearchKeepsPartialMatchesWhenTheResultOrDepthBudgetIsReached(t *testing.T) {
	remote := contentRemote(map[string]node{"/work": directory("work"), "/work/a": contentFixtureFile("a", strings.Repeat("needle\n", 201))})
	found, err := contentService(remote).Search(t.Context(), sftp.SearchOptions{Alias: "edge", Path: "/work", Query: "needle", Mode: sftp.SearchContent})
	if err != nil || len(found.Matches) != 200 || !found.Truncated {
		t.Fatalf("result limit = %+v, %v", found, err)
	}
	remote = contentRemote(map[string]node{"/work": directory("work"), "/work/a": contentFixtureFile("a", "needle")})
	current := "/work"
	for depth := 0; depth < 34; depth++ {
		current += "/deep"
		remote.nodes[current] = directory("deep")
	}
	remote.nodes[current+"/hidden"] = contentFixtureFile("hidden", "needle")
	found, err = contentService(remote).Search(t.Context(), sftp.SearchOptions{Alias: "edge", Path: "/work", Query: "needle", Mode: sftp.SearchContent})
	if err != nil || len(found.Matches) != 1 || !found.Truncated {
		t.Fatalf("depth limit = %+v, %v", found, err)
	}
}

func TestContentSearchStopsBeforeReadingMoreThan64MiB(t *testing.T) {
	remote := contentRemote(map[string]node{"/work": directory("work")})
	contents := bytes.Repeat([]byte("x"), sftp.MaxEditableFileBytes)
	for index := 0; index < 33; index++ {
		name := fmt.Sprintf("%02d.txt", index)
		remote.nodes["/work/"+name] = node{name: name, mode: 0o644, content: contents, modTime: testTime}
	}
	found, err := contentService(remote).Search(t.Context(), sftp.SearchOptions{Alias: "edge", Path: "/work", Query: "absent", Mode: sftp.SearchContent})
	if err != nil || !found.Truncated || remote.readBytes != 64<<20 || found.BytesRead != remote.readBytes {
		t.Fatalf("byte budget = %d, %+v, %v", remote.readBytes, found, err)
	}
	if len(remote.opened) != 32 {
		t.Fatalf("opened %d files beyond budget", len(remote.opened))
	}
}

func TestContentSearchDiscardsAChangedFileAndHonorsCancellation(t *testing.T) {
	for _, cancelRead := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelRead), func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			remote := contentRemote(map[string]node{"/work": directory("work"), "/work/a": contentFixtureFile("a", "needle")})
			remote.onRead = func(candidate string) {
				if cancelRead {
					cancel()
					return
				}
				changed := remote.nodes[candidate]
				changed.modTime = changed.modTime.Add(time.Second)
				remote.nodes[candidate] = changed
			}
			found, err := contentService(remote).Search(ctx, sftp.SearchOptions{Alias: "edge", Path: "/work", Query: "needle", Mode: sftp.SearchContent})
			if cancelRead {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancel = %v", err)
				}
				return
			}
			if err != nil || len(found.Matches) != 0 || !found.Truncated {
				t.Fatalf("changed = %+v, %v", found, err)
			}
		})
	}
}

func TestContentSearchDoesNotOpenAFileReplacedByASymlink(t *testing.T) {
	remote := contentRemote(map[string]node{"/work": directory("work"), "/work/a": contentFixtureFile("a", "needle"), "/outside": contentFixtureFile("outside", "needle")})
	remote.openHook = func(candidate string) { remote.nodes[candidate] = symlink("a", "/outside") }
	found, err := contentService(remote).Search(t.Context(), sftp.SearchOptions{Alias: "edge", Path: "/work", Query: "needle", Mode: sftp.SearchContent})
	if err != nil || len(found.Matches) != 0 || remote.readBytes != 0 {
		t.Fatalf("replaced link = %+v, %v; read %d", found, err, remote.readBytes)
	}
}

func TestContentComparisonDetectsDifferentBytesWithMatchingMetadata(t *testing.T) {
	left := contentRemote(map[string]node{"/work": directory("work"), "/work/a": contentFixtureFile("a", "AAAA")})
	right := contentRemote(map[string]node{"/work": directory("work"), "/work/a": contentFixtureFile("a", "BBBB")})
	service := sftp.Service{Open: func(_ context.Context, alias string) (sftp.Remote, error) {
		if alias == "left" {
			return left, nil
		}
		return right, nil
	}}
	options := sftp.CompareOptions{Left: sftp.ComparisonLocation{Alias: "left", Path: "/work"}, Right: sftp.ComparisonLocation{Alias: "right", Path: "/work"}}
	metadata, err := service.CompareDirectories(t.Context(), options)
	if err != nil || metadata.Entries[0].Status != sftp.DirectorySame || left.readBytes+right.readBytes != 0 {
		t.Fatalf("metadata = %+v, %v", metadata, err)
	}
	options.Mode = sftp.ComparisonContent
	content, err := service.CompareDirectories(t.Context(), options)
	if err != nil || content.Entries[0].Status != sftp.DirectoryDifferent || content.BytesRead != 8 {
		t.Fatalf("SHA256 = %+v, %v", content, err)
	}
}

func TestContentComparisonWorksInBothLocalRemoteDirectionsAndDetectsLocalReplacement(t *testing.T) {
	for _, localOnLeft := range []bool{true, false} {
		t.Run(fmt.Sprint(localOnLeft), func(t *testing.T) {
			folder := t.TempDir()
			localFile := filepath.Join(folder, "a")
			write := func(candidate, contents string) {
				t.Helper()
				if err := os.WriteFile(candidate, []byte(contents), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(candidate, testTime, testTime); err != nil {
					t.Fatal(err)
				}
			}
			write(localFile, "AAAA")
			remote := contentRemote(map[string]node{"/work": directory("work"), "/work/a": contentFixtureFile("a", "BBBB")})
			options := sftp.CompareOptions{Left: sftp.ComparisonLocation{Alias: "sshc://local", Path: filepath.ToSlash(folder)}, Right: sftp.ComparisonLocation{Alias: "edge", Path: "/work"}, Mode: sftp.ComparisonContent}
			if !localOnLeft {
				options.Left, options.Right = options.Right, options.Left
			}
			comparison, err := contentService(remote).CompareDirectories(t.Context(), options)
			if err != nil || comparison.Entries[0].Status != sftp.DirectoryDifferent {
				t.Fatalf("local SHA256 = %+v, %v", comparison, err)
			}
			remote.openHook = func(string) {
				write(localFile+".replacement", "AAAA")
				if err := os.Rename(localFile+".replacement", localFile); err != nil {
					t.Fatal(err)
				}
			}
			_, err = contentService(remote).CompareDirectories(t.Context(), options)
			if !errors.Is(err, sftp.ErrConflict) {
				t.Fatalf("same-metadata inode replacement = %v", err)
			}
			remote.openHook = func(string) {
				if err := os.Remove(localFile); err != nil {
					t.Fatal(err)
				}
			}
			_, err = contentService(remote).CompareDirectories(t.Context(), options)
			if !errors.Is(err, sftp.ErrConflict) {
				t.Fatalf("local file removed during comparison = %v", err)
			}
		})
	}
}

func TestContentComparisonMarksLinksAndOverBudgetFilesUnverifiedWithoutReadingThem(t *testing.T) {
	remote := contentRemote(map[string]node{"/work": directory("work"), "/work/large": contentFixtureFile("large", "bytes"), "/work/link": symlink("link", "/outside"), "/outside": contentFixtureFile("outside", "bytes")})
	remote.sizes["/work/large"] = 129 << 20
	comparison, err := contentService(remote).CompareDirectories(t.Context(), sftp.CompareOptions{Left: sftp.ComparisonLocation{Alias: "left", Path: "/work"}, Right: sftp.ComparisonLocation{Alias: "right", Path: "/work"}, Mode: sftp.ComparisonContent})
	if err != nil || !comparison.Truncated || remote.readBytes != 0 || len(comparison.Entries) != 2 {
		t.Fatalf("unverified = %+v, %v", comparison, err)
	}
	for _, entry := range comparison.Entries {
		if entry.Status != sftp.DirectoryUnverified || entry.Omission == "" {
			t.Fatalf("unchecked treated as equal: %+v", entry)
		}
	}
}

func TestContentComparisonKeepsTheParentUnverifiedWhenAnotherChildDiffers(t *testing.T) {
	remote := contentRemote(map[string]node{
		"/left": directory("left"), "/left/sub": directory("sub"),
		"/left/sub/a": contentFixtureFile("a", "AAAA"), "/left/sub/z": symlink("z", "/outside"),
		"/right": directory("right"), "/right/sub": directory("sub"),
		"/right/sub/a": contentFixtureFile("a", "BBBB"), "/right/sub/z": symlink("z", "/outside"),
		"/outside": contentFixtureFile("outside", "bytes"),
	})
	comparison, err := contentService(remote).CompareDirectories(t.Context(), sftp.CompareOptions{
		Left: sftp.ComparisonLocation{Alias: "edge", Path: "/left"}, Right: sftp.ComparisonLocation{Alias: "edge", Path: "/right"}, Mode: sftp.ComparisonContent,
	})
	if err != nil || len(comparison.Entries) != 3 || comparison.Entries[0].Status != sftp.DirectoryUnverified || comparison.Entries[1].Status != sftp.DirectoryDifferent {
		t.Fatalf("parent summary = %+v, %v", comparison, err)
	}
}

func TestContentComparisonRechecksTheFirstSideAfterHashingTheSecondSide(t *testing.T) {
	left := contentRemote(map[string]node{"/work": directory("work"), "/work/a": contentFixtureFile("a", "same")})
	right := contentRemote(map[string]node{"/work": directory("work"), "/work/a": contentFixtureFile("a", "same")})
	right.openHook = func(string) {
		changed := left.nodes["/work/a"]
		changed.modTime = changed.modTime.Add(time.Second)
		left.nodes["/work/a"] = changed
	}
	service := sftp.Service{Open: func(_ context.Context, alias string) (sftp.Remote, error) {
		if alias == "left" {
			return left, nil
		}
		return right, nil
	}}
	_, err := service.CompareDirectories(t.Context(), sftp.CompareOptions{Left: sftp.ComparisonLocation{Alias: "left", Path: "/work"}, Right: sftp.ComparisonLocation{Alias: "right", Path: "/work"}, Mode: sftp.ComparisonContent})
	if !errors.Is(err, sftp.ErrConflict) {
		t.Fatalf("first-side replacement = %v", err)
	}
}

func TestContentComparisonCancelsDuringHashingBeforeReadingTheOtherSide(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	left := contentRemote(map[string]node{"/work": directory("work"), "/work/a": contentFixtureFile("a", "same")})
	right := contentRemote(map[string]node{"/work": directory("work"), "/work/a": contentFixtureFile("a", "same")})
	left.onRead = func(string) { cancel() }
	service := sftp.Service{Open: func(_ context.Context, alias string) (sftp.Remote, error) {
		if alias == "left" {
			return left, nil
		}
		return right, nil
	}}
	_, err := service.CompareDirectories(ctx, sftp.CompareOptions{Left: sftp.ComparisonLocation{Alias: "left", Path: "/work"}, Right: sftp.ComparisonLocation{Alias: "right", Path: "/work"}, Mode: sftp.ComparisonContent})
	if !errors.Is(err, context.Canceled) || right.readBytes != 0 {
		t.Fatalf("hash cancellation = %v, other reads = %d", err, right.readBytes)
	}
}

func TestContentToolsUseTheSFTPProtocolWithoutExecutingARemoteShell(t *testing.T) {
	left, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	right, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, folder := range []string{left, right} {
		if err := os.WriteFile(filepath.Join(folder, "notes.txt"), []byte("first\nneedle 一致\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(left, "binary"), []byte{0, 1, 2, 3}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(right, "binary"), []byte{0, 1, 2, 4}, 0o644); err != nil {
		t.Fatal(err)
	}
	requests := &outstandingRequests{pending: make(map[uint32]bool)}
	opens := 0
	service := sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { opens++; return observedRemote(t, requests), nil }}
	found, err := service.Search(t.Context(), sftp.SearchOptions{Alias: "fixture", Path: servedPath(left, ""), Query: "needle", Mode: sftp.SearchContent})
	if err != nil || len(found.Matches) != 1 || found.Matches[0].Line != 2 {
		t.Fatalf("SFTP search = %+v, %v", found, err)
	}
	opens = 0
	comparison, err := service.CompareDirectories(t.Context(), sftp.CompareOptions{Left: sftp.ComparisonLocation{Alias: "fixture", Path: servedPath(left, "")}, Right: sftp.ComparisonLocation{Alias: "fixture", Path: servedPath(right, "")}, Mode: sftp.ComparisonContent})
	if err != nil || len(comparison.Entries) != 2 || comparison.Entries[0].Status != sftp.DirectoryDifferent || comparison.Entries[1].Status != sftp.DirectorySame {
		t.Fatalf("SFTP hashes = %+v, %v", comparison, err)
	}
	if opens != 1 {
		t.Fatalf("same-alias comparison opened %d transports", opens)
	}
}
