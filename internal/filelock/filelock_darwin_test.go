//go:build darwin

package filelock

import (
	"os"
	"path/filepath"
	"testing"
)

// Go の macOS test runner は /var/folders 以下に一時ディレクトリを作る。
// /var は /private/var への root 管理の互換 alias なので、利用者が状態ディレクトリへ
// 置いた symlink とは区別して lock を取得できなければならない。
func TestTryAcquireThroughMacOSSystemVarAlias(t *testing.T) {
	info, err := os.Lstat("/var")
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Skip("this macOS installation does not expose /var as a symlink")
	}
	path := filepath.Join(t.TempDir(), "engine.lock")
	release, err := TryAcquire(path)
	if err != nil {
		t.Fatalf("TryAcquire(%q) = %v", path, err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
}
