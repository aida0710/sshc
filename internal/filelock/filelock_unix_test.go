//go:build unix

package filelock

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// ロックファイルは秘密を持たないが、engine 所有権の証拠であり、別のユーザーが書ける
// 場所に置けば所有の直列化そのものを歪められる。既に緩い状態で残っていても、
// 取得時に締め直す。
func TestTryAcquireTightensLooseUnixPrivateState(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "state")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "engine.lock")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}

	release, err := TryAcquire(path)
	if err != nil {
		t.Fatalf("TryAcquire = %v", err)
	}
	defer func() {
		if releaseErr := release(); releaseErr != nil {
			t.Fatal(releaseErr)
		}
	}()

	for path, want := range map[string]fs.FileMode{directory: 0o700, path: 0o600} {
		info, statErr := os.Lstat(path)
		if statErr != nil {
			t.Fatal(statErr)
		}
		if got := info.Mode().Perm(); got != want {
			t.Fatalf("%s mode = %v, want %v", filepath.Base(path), got, want)
		}
	}
}

// リンク越しに締め直すと、誰が差し替えたか分からない先の権限を書き換えることに
// なる。ロックの置き場所がディレクトリそのものでないなら、そこには置かない。
func TestTryAcquireRefusesAStateDirectoryThatIsASymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "state")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	release, err := TryAcquire(filepath.Join(link, "engine.lock"))
	if !errors.Is(err, ErrUnsafeDirectory) {
		t.Fatalf("TryAcquire through a symlinked state directory = %v, want ErrUnsafeDirectory", err)
	}
	if release != nil {
		t.Fatal("a refused TryAcquire returned a release function")
	}
	info, statErr := os.Lstat(target)
	if statErr != nil {
		t.Fatal(statErr)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("the symlink target was tightened to %v", info.Mode().Perm())
	}
}

func TestTryAcquireRefusesALockFileThatIsASymlink(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "state")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "unrelated")
	if err := os.WriteFile(target, []byte("unchanged"), 0o644); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(directory, "engine.lock")
	if err := os.Symlink(target, lockPath); err != nil {
		t.Fatal(err)
	}

	release, err := TryAcquire(lockPath)
	if err == nil {
		if release != nil {
			_ = release()
		}
		t.Fatal("TryAcquire followed a symlinked lock file")
	}
	if release != nil {
		t.Fatal("a refused TryAcquire returned a release function")
	}
	contents, readErr := os.ReadFile(target)
	if readErr != nil || string(contents) != "unchanged" {
		t.Fatalf("symlink target contents = %q, %v", contents, readErr)
	}
	info, statErr := os.Stat(target)
	if statErr != nil {
		t.Fatal(statErr)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("symlink target mode = %v", info.Mode().Perm())
	}
}

func TestTryAcquirePinsTheDirectoryBeforeAPathReplacement(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "state")
	path := filepath.Join(directory, "engine.lock")
	first, err := TryAcquire(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first() }()

	moved := filepath.Join(root, "state-original")
	outside := filepath.Join(root, "outside")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	afterLockDirectoryOpen = func() {
		afterLockDirectoryOpen = nil
		if renameErr := os.Rename(directory, moved); renameErr != nil {
			t.Fatal(renameErr)
		}
		if symlinkErr := os.Symlink(outside, directory); symlinkErr != nil {
			t.Fatal(symlinkErr)
		}
	}
	t.Cleanup(func() { afterLockDirectoryOpen = nil })

	second, err := TryAcquire(path)
	if !errors.Is(err, ErrHeld) {
		if second != nil {
			_ = second()
		}
		t.Fatalf("TryAcquire across directory replacement = %v, want ErrHeld", err)
	}
	if second != nil {
		t.Fatal("a contended TryAcquire returned a release function")
	}
	if _, statErr := os.Lstat(filepath.Join(outside, "engine.lock")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("replacement target received a lock file: %v", statErr)
	}
}
