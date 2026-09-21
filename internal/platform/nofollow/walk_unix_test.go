//go:build !windows

package nofollow

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
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
