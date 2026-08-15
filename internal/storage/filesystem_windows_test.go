//go:build windows

package storage

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
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

func TestOSFileSystemReadFileAcceptsDriveCaseAndShortNames(t *testing.T) {
	directory := t.TempDir()
	if strings.HasPrefix(directory, `\\`) {
		t.Skipf("local-drive test requires a drive-letter temp directory, got %q", directory)
	}
	target := filepath.Join(directory, "Mixed Case File.txt")
	if err := os.WriteFile(target, []byte("Host case-test\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("drive and case", func(t *testing.T) {
		request := filepath.Join(directory, "mixed case file.txt")
		contents, err := (OSFileSystem{}).ReadFile(request)
		if err != nil {
			t.Fatalf("ReadFile(%q) = %v", request, err)
		}
		if string(contents) != "Host case-test\n" {
			t.Fatalf("contents = %q, want %q", contents, "Host case-test\n")
		}
	})
	t.Run("8.3 short name", func(t *testing.T) {
		shortPath := windowsShortPath(t, target)
		if strings.EqualFold(shortPath, target) {
			t.Skipf("8.3 short aliases are unavailable for %q", target)
		}
		contents, err := (OSFileSystem{}).ReadFile(shortPath)
		if err != nil {
			t.Fatalf("ReadFile(%q) = %v", shortPath, err)
		}
		if string(contents) != "Host case-test\n" {
			t.Fatalf("contents = %q, want %q", contents, "Host case-test\n")
		}
	})
}

func TestOSFileSystemReadFileAcceptsConfiguredUNCPath(t *testing.T) {
	path := os.Getenv("SSHC_WINDOWS_TEST_UNC_FILE")
	if path == "" {
		t.Skip("SSHC_WINDOWS_TEST_UNC_FILE is not configured")
	}
	if !strings.HasPrefix(path, `\\`) {
		t.Fatalf("SSHC_WINDOWS_TEST_UNC_FILE = %q, want UNC path", path)
	}
	contents, err := (OSFileSystem{}).ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) = %v", path, err)
	}
	if len(contents) == 0 {
		t.Fatal("ReadFile returned empty content for configured UNC fixture")
	}
}

func windowsShortPath(t *testing.T, path string) string {
	t.Helper()
	pathUTF16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	buffer := make([]uint16, 260)
	for {
		n, err := windows.GetShortPathName(pathUTF16, &buffer[0], uint32(len(buffer)))
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			t.Fatalf("GetShortPathName(%q) returned no path", path)
		}
		if n < uint32(len(buffer)) {
			return windows.UTF16ToString(buffer)
		}
		buffer = make([]uint16, n+1)
	}
}
