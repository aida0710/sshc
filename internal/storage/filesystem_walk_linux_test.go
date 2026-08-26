//go:build linux

package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOSFileSystemTraversesAncestorsWithoutDirectoryReadPermission(t *testing.T) {
	base := t.TempDir()
	traversalOnly := filepath.Join(base, "traversal-only")
	home := filepath.Join(traversalOnly, "app", "files")
	if err := os.MkdirAll(home, DirectoryPermission); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(traversalOnly, 0o111); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(traversalOnly, DirectoryPermission) })

	state := filepath.Join(home, ".ssh", "sshc")
	if err := (OSFileSystem{}).MkdirAll(state, DirectoryPermission); err != nil {
		t.Fatalf("MkdirAll through traversal-only ancestor = %v", err)
	}
	config := filepath.Join(home, ".ssh", "config")
	if err := os.WriteFile(config, []byte("Host android\n"), FilePermission); err != nil {
		t.Fatal(err)
	}
	contents, err := (OSFileSystem{}).ReadFile(config)
	if err != nil {
		t.Fatalf("ReadFile through traversal-only ancestor = %v", err)
	}
	if string(contents) != "Host android\n" {
		t.Fatalf("ReadFile contents = %q", contents)
	}
}
