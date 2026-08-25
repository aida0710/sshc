//go:build !windows

package sftp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadSpoolUsesAnUnpredictablePrivateDirectory(t *testing.T) {
	temporaryRoot := t.TempDir()
	outside := t.TempDir()
	predictable := filepath.Join(temporaryRoot, "sshc-sftp-spool")
	if err := os.Symlink(outside, predictable); err != nil {
		t.Fatal(err)
	}

	root := createDownloadSpoolDirectory(temporaryRoot)
	if root == "" {
		t.Fatal("createDownloadSpoolDirectory returned no directory")
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
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
