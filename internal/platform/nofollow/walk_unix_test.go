//go:build !windows

package nofollow

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestASymlinkAnywhereOnThePathIsRefused(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.MkdirAll(filepath.Join(real, "inner"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "inner", "file"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}

	if _, err := OpenRegular(filepath.Join(link, "inner", "file")); !errors.Is(err, ErrSymlinkPath) {
		t.Fatalf("OpenRegular through a symlinked parent = %v, want ErrSymlinkPath", err)
	}
	if _, err := OpenDirectory(link); !errors.Is(err, ErrSymlinkPath) {
		t.Fatalf("OpenDirectory of a symlink = %v, want ErrSymlinkPath", err)
	}
	if _, err := OpenOrCreateDirectory(filepath.Join(link, "new"), 0o700); !errors.Is(err, ErrSymlinkPath) {
		t.Fatalf("OpenOrCreateDirectory below a symlink = %v, want ErrSymlinkPath", err)
	}
}

func TestOpenOrCreateDirectoryCreatesTheMissingTailWithThePermission(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "a", "b", "c")

	directory, err := OpenOrCreateDirectory(target, 0o700)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("created %v with mode %v, want a 0700 directory", info.IsDir(), info.Mode().Perm())
	}
	// 返った descriptor はその directory を指す: 相対に作ったファイルが見える。
	if err := os.WriteFile(filepath.Join(target, "child"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := directory.Stat(); err != nil {
		t.Fatal(err)
	}
}

func TestARelativePathAndTheRootItselfAreRefused(t *testing.T) {
	for _, path := range []string{"relative/path", "/"} {
		if _, err := OpenOrCreateDirectory(path, 0o700); err == nil {
			t.Fatalf("OpenOrCreateDirectory(%q) succeeded, want an error", path)
		}
	}
}

// fifoOpenDeadline は、FIFO を開く呼び出しが戻るまで待つ上限。open はすぐに戻る
// はずなので、遅いテスト環境でも誤って落ちない程度に余裕を持たせる。
const fifoOpenDeadline = 5 * time.Second

// 保留記録の対象が FIFO に置き換わっても、読み手は書き手を待って止まらない。
// 止まると、engine の起動や設定画面が戻らなくなる。
func TestOpenRegularReturnsForAFIFOWithoutWaitingForAWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fifo")
	if err := unix.Mkfifo(path, 0o600); err != nil {
		t.Skipf("FIFOs are not available: %v", err)
	}
	opened := make(chan os.FileMode, 1)
	go func() {
		file, err := OpenRegular(path)
		if err != nil {
			opened <- 0
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			opened <- 0
			return
		}
		opened <- info.Mode()
	}()
	select {
	case mode := <-opened:
		if mode.IsRegular() {
			t.Fatalf("a FIFO was reported as a regular file: %v", mode)
		}
	case <-time.After(fifoOpenDeadline):
		t.Fatal("OpenRegular on a FIFO is still waiting for a writer")
	}
}
