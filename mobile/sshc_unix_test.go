//go:build !windows

package mobile

import (
	"os"
	"path/filepath"
	"testing"
)

// Androidがprivate directoryを別名経由で返しても起動できることを確かめる。
// 正規化するのはframeworkから受け取ったrootだけで、その配下の検査は緩めない。
func TestStartAcceptsFrameworkPrivateDirectoriesThroughAliases(t *testing.T) {
	root := t.TempDir()
	realHome := filepath.Join(root, "private-home")
	realCache := filepath.Join(root, "private-cache")
	if err := os.Mkdir(realHome, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(realCache, 0o700); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "framework-home")
	cache := filepath.Join(root, "framework-cache")
	if err := os.Symlink(realHome, home); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realCache, cache); err != nil {
		t.Fatal(err)
	}
	url, err := Start(home, cache)
	if err != nil {
		t.Fatalf("Start through framework aliases = %v", err)
	}
	t.Cleanup(func() { _ = Stop() })
	if url == "" {
		t.Fatal("Start returned no entrance")
	}
}

func TestUnsafePrivateStateIsNotReportedAsAnotherEngine(t *testing.T) {
	home := t.TempDir()
	cache := t.TempDir()
	ssh := filepath.Join(home, ".ssh")
	if err := os.Mkdir(ssh, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(ssh, "sshc")); err != nil {
		t.Fatal(err)
	}
	if _, err := Start(home, cache); err == nil {
		t.Fatal("Start accepted a linked private state directory")
	}
	if got := LastStartFailureKind(); got != KindStorageUnavailable {
		t.Fatalf("LastStartFailureKind() = %d, want %d", got, KindStorageUnavailable)
	}
}
