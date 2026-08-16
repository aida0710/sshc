//go:build unix

package application

import (
	"os"
	"testing"
)

// markKeyMode と assertKeyModeSurvived は、移動が rename(2) であることを
// permission bit で確かめる。バイト列がこのプロセスを通って運ばれれば、
// 新しいファイルは新しい mode を持つ——0400 が生き残ったことが、運ばれて
// いない証拠である。
func markKeyMode(t *testing.T, path string) {
	t.Helper()
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatal(err)
	}
}

func assertKeyModeSurvived(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("relocated key missing: %v", err)
	}
	if info.Mode().Perm() != 0o400 {
		t.Errorf("permission = %04o, want the original 0400", info.Mode().Perm())
	}
}
