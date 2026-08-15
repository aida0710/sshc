//go:build !windows

package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOSFileSystemReadFileRefusesSymlink(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "config")
	if err := os.WriteFile(target, []byte("Host example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	if _, err := (OSFileSystem{}).ReadFile(link); !errors.Is(err, ErrSymlinkPath) {
		t.Fatalf("ReadFile(symlink) error = %v, want ErrSymlinkPath", err)
	}
}

func TestOSFileSystemWriteTempAppliesPOSIXPermission(t *testing.T) {
	path, err := (OSFileSystem{}).WriteTemp(t.TempDir(), ".sshc-", FilePermission, []byte("staged"))
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != FilePermission {
		t.Fatalf("permission = %v, want %v", info.Mode().Perm(), FilePermission)
	}
}
