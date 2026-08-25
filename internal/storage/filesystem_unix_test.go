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

func TestWriteAtomicFilePinsParentBeforeSymlinkReplacement(t *testing.T) {
	base := t.TempDir()
	parent := filepath.Join(base, "state")
	original := filepath.Join(base, "state-original")
	outside := filepath.Join(base, "outside")
	for _, directory := range []string{parent, outside} {
		if err := os.Mkdir(directory, DirectoryPermission); err != nil {
			t.Fatal(err)
		}
	}
	target := filepath.Join(parent, "vault")
	if err := os.WriteFile(target, []byte("old"), FilePermission); err != nil {
		t.Fatal(err)
	}

	err := writeAtomicFileNativeWith(target, ".vault-", FilePermission, []byte("sealed"), func() {
		if err := os.Rename(parent, original); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, parent); err != nil {
			t.Fatal(err)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(outside, "vault")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("outside target error = %v, want not exist", err)
	}
	contents, err := os.ReadFile(filepath.Join(original, "vault"))
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "sealed" {
		t.Fatalf("pinned target contents = %q, want sealed", contents)
	}
}
