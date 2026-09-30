package sftp_test

import (
	"context"
	"errors"
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

// caseInsensitiveRemote resolves names the way APFS and NTFS do by default: a
// path that differs from an existing entry only by case names that entry.
type caseInsensitiveRemote struct{ *fakeRemote }

func (r caseInsensitiveRemote) resolve(candidate string) string {
	if candidate == "/" {
		return candidate
	}
	parent := r.resolve(path.Dir(candidate))
	joined := path.Join(parent, path.Base(candidate))
	if _, ok := r.nodes[joined]; ok {
		return joined
	}
	for existing := range r.nodes {
		if path.Dir(existing) == parent && strings.EqualFold(path.Base(existing), path.Base(candidate)) {
			return existing
		}
	}
	return joined
}

func (r caseInsensitiveRemote) Close() error { return nil }
func (r caseInsensitiveRemote) ReadDir(ctx context.Context, directory string) ([]fs.FileInfo, error) {
	return r.fakeRemote.ReadDir(ctx, r.resolve(directory))
}
func (r caseInsensitiveRemote) Lstat(candidate string) (fs.FileInfo, error) {
	return r.fakeRemote.Lstat(r.resolve(candidate))
}
func (r caseInsensitiveRemote) Open(candidate string) (io.ReadCloser, error) {
	return r.fakeRemote.Open(r.resolve(candidate))
}
func (r caseInsensitiveRemote) Create(candidate string) (io.WriteCloser, error) {
	return r.fakeRemote.Create(r.resolve(candidate))
}
func (r caseInsensitiveRemote) Mkdir(candidate string) error {
	return r.fakeRemote.Mkdir(r.resolve(candidate))
}
func (r caseInsensitiveRemote) Chmod(candidate string, mode fs.FileMode) error {
	return r.fakeRemote.Chmod(r.resolve(candidate), mode)
}
func (r caseInsensitiveRemote) Chtimes(candidate string, modified time.Time) error {
	return r.fakeRemote.Chtimes(r.resolve(candidate), modified)
}
func (r caseInsensitiveRemote) Replace(from, to string) error {
	return r.fakeRemote.Replace(r.resolve(from), r.resolve(to))
}
func (r caseInsensitiveRemote) Rename(from, to string) error {
	return r.fakeRemote.Rename(r.resolve(from), r.resolve(to))
}
func (r caseInsensitiveRemote) Remove(candidate string) error {
	return r.fakeRemote.Remove(r.resolve(candidate))
}
func (r caseInsensitiveRemote) RemoveDirectory(candidate string) error {
	return r.fakeRemote.RemoveDirectory(r.resolve(candidate))
}

// caseVariantTree is a source folder holding two files whose names differ
// only by case, as the Linux kernel's xt_CONNMARK.h and xt_connmark.h do.
func caseVariantTree() *fakeRemote {
	return remoteWith(map[string]node{
		"/data":       directory("data"),
		"/data/A.txt": file("A.txt", "upper", 0o644),
		"/data/a.txt": file("a.txt", "lower", 0o644),
	})
}

func twoHostService(source, target sftp.Remote) sftp.Service {
	return sftp.Service{Open: func(_ context.Context, alias string) (sftp.Remote, error) {
		if alias == "source" {
			return source, nil
		}
		return target, nil
	}}
}

func TestCopyIntoACaseInsensitiveTargetStopsOnANameCollisionEvenWithOverwriteApproved(t *testing.T) {
	source := caseVariantTree()
	target := caseInsensitiveRemote{remoteWith(map[string]node{"/inbox": directory("inbox")})}
	service := twoHostService(fakeConnection{source}, target)
	for _, overwrite := range []bool{false, true} {
		err := service.CopyRemote(context.Background(), sftp.RemoteTransferRequest{
			SourceAlias: "source", SourcePath: "/data", TargetAlias: "target", TargetPath: "/inbox/data",
			Operation: sftp.RemoteCopy, Overwrite: overwrite,
		}, nil)
		if !errors.Is(err, sftp.ErrNameCollision) {
			t.Fatalf("overwrite=%v: err = %v, want a name collision rather than an overwrite prompt", overwrite, err)
		}
	}
	if got := string(target.nodes["/inbox/data/A.txt"].content); got != "upper" {
		t.Fatalf("A.txt = %q; the second name must not replace what the run wrote", got)
	}
}

func TestMoveIntoACaseInsensitiveTargetKeepsBothSourceFilesOnANameCollision(t *testing.T) {
	source := caseVariantTree()
	target := caseInsensitiveRemote{remoteWith(map[string]node{"/inbox": directory("inbox")})}
	service := twoHostService(fakeConnection{source}, target)
	err := service.CopyRemote(context.Background(), sftp.RemoteTransferRequest{
		SourceAlias: "source", SourcePath: "/data", TargetAlias: "target", TargetPath: "/inbox/data",
		Operation: sftp.RemoteMove, Overwrite: true,
	}, nil)
	if !errors.Is(err, sftp.ErrNameCollision) {
		t.Fatalf("move err = %v", err)
	}
	if string(source.nodes["/data/A.txt"].content) != "upper" || string(source.nodes["/data/a.txt"].content) != "lower" {
		t.Fatalf("source after the failed move = %+v", source.nodes)
	}
}

func TestPutIntoACaseInsensitiveTargetStopsOnANameCollision(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	local := filepath.Join(home, "data")
	if err := os.Mkdir(local, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string]string{"A.txt": "upper", "a.txt": "lower"} {
		if err := os.WriteFile(filepath.Join(local, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// APFS and NTFS fold case by default, so there the second write above
	// lands in the first file and no pair of names is left to put. The rule
	// itself is checked on every system by the copy and move tests above;
	// only put's use of it needs a case-sensitive local file system.
	if entries, err := os.ReadDir(local); err != nil {
		t.Fatal(err)
	} else if len(entries) != 2 {
		t.Skip("the local file system folds case, so a folder cannot hold both A.txt and a.txt")
	}
	target := caseInsensitiveRemote{remoteWith(map[string]node{"/inbox": directory("inbox")})}
	service := sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return target, nil }}
	for _, overwrite := range []bool{false, true} {
		err := service.CopyLocal(context.Background(), sftp.RemoteTransferRequest{
			SourcePath: filepath.ToSlash(local), TargetAlias: "target", TargetPath: "/inbox/data",
			Operation: sftp.RemotePut, Overwrite: overwrite,
		}, nil)
		if !errors.Is(err, sftp.ErrNameCollision) {
			t.Fatalf("overwrite=%v: err = %v", overwrite, err)
		}
	}
}

func TestMoveKeepsASourceFileWhosePublishedCopyWasReplacedBeforeTheSourceIsRemoved(t *testing.T) {
	source := remoteWith(map[string]node{"/report.txt": file("report.txt", "original", 0o644)})
	target := remoteWith(map[string]node{"/inbox": directory("inbox")})
	publishedLookups := 0
	target.lstatHook = func(candidate string) error {
		published, exists := target.nodes[candidate]
		if candidate != "/inbox/report.txt" || !exists {
			return nil
		}
		// The first lookup of the published copy records its revision; by the
		// second one, right before the source is removed, someone replaced it.
		publishedLookups++
		if publishedLookups == 2 {
			published.content = []byte("someone else's")
			published.modTime = testTime.Add(time.Hour)
			target.nodes[candidate] = published
		}
		return nil
	}
	service := twoHostService(fakeConnection{source}, fakeConnection{target})
	err := service.CopyRemote(context.Background(), sftp.RemoteTransferRequest{
		SourceAlias: "source", SourcePath: "/report.txt", TargetAlias: "target", TargetPath: "/inbox/report.txt",
		Operation: sftp.RemoteMove,
	}, nil)
	if !errors.Is(err, sftp.ErrConflict) {
		t.Fatalf("move err = %v, want a conflict", err)
	}
	if string(source.nodes["/report.txt"].content) != "original" {
		t.Fatal("the source was removed although the target no longer holds its copy")
	}
}

func TestQueuedCopyReportsANameCollisionInsteadOfAskingToOverwrite(t *testing.T) {
	source := caseVariantTree()
	target := caseInsensitiveRemote{remoteWith(map[string]node{"/inbox": directory("inbox")})}
	service := twoHostService(fakeConnection{source}, target)
	manager := newTestTransferManager(t, &service)
	t.Cleanup(func() { _ = manager.Close() })
	if _, err := manager.CreateJob(sftp.CreateTransferJob{
		ID: "case_collision", BatchID: "batch_case_collision", Alias: "target", RemotePath: "/inbox/data",
		SourceAlias: "source", SourcePath: "/data", Operation: sftp.RemoteCopy,
		Direction: sftp.TransferRemote, Kind: sftp.TransferFolder, Name: "data", TotalBytes: -1,
	}); err != nil {
		t.Fatal(err)
	}
	job := waitForJob(t, manager, "case_collision", func(job sftp.TransferJob) bool {
		return job.Status == sftp.TransferFailed || job.Status == sftp.TransferNeedsOverwrite
	})
	if job.Status != sftp.TransferFailed || job.Problem != "sftp_name_collision" {
		t.Fatalf("job = %s/%s, want failed/sftp_name_collision", job.Status, job.Problem)
	}
}
