package storage

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOSFileSystemReadFileRejectsNonRegularAndOversizedFiles(t *testing.T) {
	directory := t.TempDir()
	fileSystem := OSFileSystem{}

	target := filepath.Join(directory, "config")
	if err := os.WriteFile(target, []byte("Host example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	contents, err := fileSystem.ReadFile(target)
	if err != nil || !bytes.Equal(contents, []byte("Host example\n")) {
		t.Fatalf("ReadFile = %q, %v", contents, err)
	}

	if _, err := fileSystem.ReadFile(directory); !errors.Is(err, ErrNotRegularFile) {
		t.Fatalf("ReadFile(directory) error = %v, want ErrNotRegularFile", err)
	}

	oversized := filepath.Join(directory, "big")
	if err := os.WriteFile(oversized, bytes.Repeat([]byte("x"), MaxFileSize+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fileSystem.ReadFile(oversized); !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("ReadFile(oversized) error = %v, want ErrFileTooLarge", err)
	}
}

func TestOSFileSystemWriteTempLeavesNoFileWhenTargetIsNotDirectory(t *testing.T) {
	directory := t.TempDir()
	notDirectory := filepath.Join(directory, "not-a-directory")
	if err := os.WriteFile(notDirectory, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := (OSFileSystem{}).WriteTemp(notDirectory, ".sshc-", FilePermission, []byte("staged")); err == nil {
		t.Fatal("WriteTemp unexpectedly succeeded")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "not-a-directory" {
		t.Fatalf("directory entries = %v, want only %q", entries, "not-a-directory")
	}
}

func TestOSFileSystemRenameReplacesExistingFile(t *testing.T) {
	directory := t.TempDir()
	oldPath := filepath.Join(directory, "old")
	newPath := filepath.Join(directory, "new")
	if err := os.WriteFile(oldPath, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, []byte("previous"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := (OSFileSystem{}).Rename(oldPath, newPath); err != nil {
		t.Fatalf("Rename = %v", err)
	}
	contents, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "replacement" {
		t.Fatalf("replacement contents = %q, want %q", contents, "replacement")
	}
	if _, err := os.Lstat(oldPath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("old path error = %v, want fs.ErrNotExist", err)
	}
}

func TestOSFileSystemWriteTempCreatesPrivateFileInTargetDirectory(t *testing.T) {
	directory := t.TempDir()
	path, err := OSFileSystem{}.WriteTemp(directory, ".sshc-", FilePermission, []byte("staged"))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != directory || !strings.HasPrefix(filepath.Base(path), ".sshc-") {
		t.Fatalf("temp path = %q", path)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != FilePermission {
		t.Fatalf("permission = %v, want %v", info.Mode().Perm(), FilePermission)
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "staged" {
		t.Fatalf("contents = %q, %v", contents, err)
	}
}

func TestOSFileSystemSyncDirAndGlobAreLexical(t *testing.T) {
	directory := t.TempDir()
	for _, name := range []string{"20-b.conf", "10-a.conf"} {
		if err := os.WriteFile(filepath.Join(directory, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	matches, err := OSFileSystem{}.Glob(filepath.Join(directory, "*.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 || filepath.Base(matches[0]) != "10-a.conf" {
		t.Fatalf("matches = %#v", matches)
	}
	if err := (OSFileSystem{}).SyncDir(directory); err != nil {
		t.Fatalf("SyncDir = %v", err)
	}
	if _, err := (OSFileSystem{}).Lstat(filepath.Join(directory, "missing")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Lstat(missing) error = %v", err)
	}
}
