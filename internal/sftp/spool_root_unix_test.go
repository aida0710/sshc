//go:build !windows

package sftp

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAMissingSpoolRootIsCreatedAsAPrivateDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "cache", "sshc", "sftp-spool")
	if err := prepareDownloadSpoolRoot(root); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(root)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("spool root mode = %v, want a 0700 directory", info.Mode())
	}
}

func TestSpoolRootNarrowsAnExistingWiderDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sftp-spool")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := prepareDownloadSpoolRoot(root); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(root)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("spool root mode = %v, want 0700", info.Mode().Perm())
	}
}

func TestSpoolRootRefusesASymlink(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sftp-spool")
	if err := os.Symlink(t.TempDir(), root); err != nil {
		t.Fatal(err)
	}
	if err := prepareDownloadSpoolRoot(root); !errors.Is(err, errSpoolRootNotPrivate) {
		t.Fatalf("spool root through a symlink = %v, want %v", err, errSpoolRootNotPrivate)
	}
}

// A quota lock that cannot be opened never clears by waiting, so it must not
// look like a full quota to the browser or the CLI, which retry that.
func TestAnUnusableQuotaLockMakesTheSpoolUnavailableRatherThanFull(t *testing.T) {
	tests := []struct {
		name  string
		place func(t *testing.T, lockPath string)
	}{
		{name: "symlink", place: func(t *testing.T, lockPath string) {
			target := filepath.Join(t.TempDir(), "elsewhere.lock")
			if err := os.WriteFile(target, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, lockPath); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "file readable by others", place: func(t *testing.T, lockPath string) {
			if err := os.WriteFile(lockPath, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(lockPath, 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "directory", place: func(t *testing.T, lockPath string) {
			if err := os.Mkdir(lockPath, 0o700); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			current := filepath.Join(root, "sshc-sftp-spool-current")
			if err := os.Mkdir(current, 0o700); err != nil {
				t.Fatal(err)
			}
			test.place(t, downloadSpoolQuotaPath(root))
			_, err := reserveDownloadSpool(root, current, 0, 1, 10)
			if !errors.Is(err, ErrSpoolUnavailable) || errors.Is(err, ErrTransferLimit) {
				t.Fatalf("reserve with a %s at the quota lock = %v, want only %v", test.name, err, ErrSpoolUnavailable)
			}
		})
	}
}
