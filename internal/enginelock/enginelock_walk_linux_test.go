//go:build linux

package enginelock

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAcquireTraversesAncestorWithoutDirectoryReadPermission(t *testing.T) {
	base := t.TempDir()
	traversalOnly := filepath.Join(base, "traversal-only")
	state := filepath.Join(traversalOnly, "app", "files", ".ssh", "sshc")
	if err := os.MkdirAll(state, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(traversalOnly, 0o111); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(traversalOnly, 0o700) })

	release, err := Acquire(filepath.Join(state, "mutation.lock"))
	if err != nil {
		t.Fatalf("Acquire through traversal-only ancestor = %v", err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
}
