//go:build linux

package mobile

import (
	"os"
	"path/filepath"
	"testing"
)

// Android 16では/や/dataの一覧を読む権限がなくても、frameworkから渡された
// app private directoryへは既知pathとして到達できる。この権限模型をLinux上で
// execute-only ancestorとして再現し、engine全体が起動することを固定する。
func TestStartTraversesAndroidPrivateAncestorsWithoutReadingThem(t *testing.T) {
	base := t.TempDir()
	traversalOnly := filepath.Join(base, "android-data")
	home := filepath.Join(traversalOnly, "user", "0", "sshc", "files")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(traversalOnly, 0o111); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(traversalOnly, 0o700) })

	url, err := Start(home, t.TempDir())
	if err != nil {
		t.Fatalf("Start through traversal-only Android ancestors = %v", err)
	}
	t.Cleanup(func() { _ = Stop() })
	if url == "" {
		t.Fatal("Start returned no entrance")
	}
}
