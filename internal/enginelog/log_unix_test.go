//go:build !windows

package enginelog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLogAndDirectoryArePrivate(t *testing.T) {
	stateDir := t.TempDir()
	file, err := Open(stateDir, "test")
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	for path, want := range map[string]os.FileMode{Path(stateDir): 0o600, filepath.Dir(Path(stateDir)): 0o700} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != want {
			t.Fatalf("permissions for %s: %v, %v; want %o", path, info, err, want)
		}
	}
}

func TestLogDoesNotFollowASymlink(t *testing.T) {
	stateDir := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Dir(Path(stateDir)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, Path(stateDir)); err != nil {
		t.Fatal(err)
	}
	if file, err := Open(stateDir, "test"); err == nil {
		file.Close()
		t.Fatal("followed a log symlink")
	}
	body, err := os.ReadFile(target)
	if err != nil || string(body) != "untouched" {
		t.Fatalf("target = %q, %v", body, err)
	}
}
