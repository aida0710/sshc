//go:build unix

package enginelock

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// ロックファイルは秘密を持たないが、engine 所有権の証拠であり、他人が書ける
// 場所に置けば所有の直列化そのものを歪められる。既に緩い状態で残っていても、
// 取得時に締め直す。
func TestAcquireTightensLooseUnixPrivateState(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "state")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "engine.lock")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}

	release, err := Acquire(path)
	if err != nil {
		t.Fatalf("Acquire = %v", err)
	}
	defer func() {
		if releaseErr := release(); releaseErr != nil {
			t.Fatal(releaseErr)
		}
	}()

	for path, want := range map[string]fs.FileMode{directory: 0o700, path: 0o600} {
		info, statErr := os.Lstat(path)
		if statErr != nil {
			t.Fatal(statErr)
		}
		if got := info.Mode().Perm(); got != want {
			t.Fatalf("%s mode = %v, want %v", filepath.Base(path), got, want)
		}
	}
}
