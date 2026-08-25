//go:build darwin

package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOSFileSystemWorksThroughMacOSSystemVarAlias(t *testing.T) {
	info, err := os.Lstat("/var")
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Skip("this macOS installation does not expose /var as a symlink")
	}
	directory := t.TempDir()
	fileSystem := OSFileSystem{}
	if err := fileSystem.MkdirAll(filepath.Join(directory, "private"), DirectoryPermission); err != nil {
		t.Fatalf("MkdirAll through /var: %v", err)
	}
	target := filepath.Join(directory, "private", "config")
	if err := WriteAtomicFile(fileSystem, target, ".config-", FilePermission, []byte("Host macOS\n")); err != nil {
		t.Fatalf("WriteAtomicFile through /var: %v", err)
	}
	contents, err := fileSystem.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile through /var: %v", err)
	}
	if string(contents) != "Host macOS\n" {
		t.Fatalf("contents = %q", contents)
	}
}
