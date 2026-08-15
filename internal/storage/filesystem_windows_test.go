//go:build windows

package storage

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestOSFileSystemReadFileRefusesWindowsReparsePoints(t *testing.T) {
	directory := t.TempDir()
	fileTarget := filepath.Join(directory, "target")
	if err := os.WriteFile(fileTarget, []byte("Host example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	directoryTarget := filepath.Join(directory, "target-directory")
	if err := os.Mkdir(directoryTarget, 0o700); err != nil {
		t.Fatal(err)
	}
	insideDirectoryTarget := filepath.Join(directoryTarget, "config")
	if err := os.WriteFile(insideDirectoryTarget, []byte("Host inside-directory\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("file symlink", func(t *testing.T) {
		link := filepath.Join(directory, "file-symlink")
		if err := os.Symlink(fileTarget, link); err != nil {
			t.Skipf("Windows symlink creation is unavailable: %v", err)
		}
		assertReadFileRejectsReparsePoint(t, link)
	})
	t.Run("directory symlink", func(t *testing.T) {
		link := filepath.Join(directory, "directory-symlink")
		if err := os.Symlink(directoryTarget, link); err != nil {
			t.Skipf("Windows directory symlink creation is unavailable: %v", err)
		}
		assertReadFileRejectsReparsePoint(t, link)
		assertReadFileRejectsReparsePoint(t, filepath.Join(link, "config"))
	})
	t.Run("junction", func(t *testing.T) {
		link := filepath.Join(directory, "junction")
		if err := exec.Command("cmd.exe", "/c", "mklink", "/J", link, directoryTarget).Run(); err != nil {
			t.Skipf("Windows junction creation is unavailable: %v", err)
		}
		assertReadFileRejectsReparsePoint(t, link)
		assertReadFileRejectsReparsePoint(t, filepath.Join(link, "config"))
	})
}

func assertReadFileRejectsReparsePoint(t *testing.T, path string) {
	t.Helper()
	if _, err := (OSFileSystem{}).ReadFile(path); !errors.Is(err, ErrSymlinkPath) {
		t.Fatalf("ReadFile(%q) error = %v, want ErrSymlinkPath", path, err)
	}
}
