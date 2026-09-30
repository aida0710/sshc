//go:build !windows

package sftp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadSpoolUsesAnUnpredictablePrivateDirectory(t *testing.T) {
	spoolRoot := t.TempDir()
	outside := t.TempDir()
	predictable := filepath.Join(spoolRoot, "sshc-sftp-spool")
	if err := os.Symlink(outside, predictable); err != nil {
		t.Fatal(err)
	}

	root, owner, err := createDownloadSpoolDirectory(spoolRoot)
	if err != nil {
		t.Fatalf("createDownloadSpoolDirectory() = %v", err)
	}
	t.Cleanup(func() { _ = owner.Close() })
	if root == predictable {
		t.Fatal("download spool reused the attacker-controlled predictable path")
	}
	info, err := os.Lstat(root)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("download spool mode = %v, want a real directory", info.Mode())
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("download spool permission = %v, want 0700", info.Mode().Perm())
	}
}
