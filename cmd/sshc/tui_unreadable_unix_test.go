//go:build unix

package main

import (
	"os"
	"testing"
)

// denyConfigRead は、いまの利用者から ~/.ssh/config の読み取りを取り上げる。
//
// Unix ではファイルの mode がそのままアクセス制御なので、0000 がその最小の形で
// ある。Windows には同じ意味の mode が無く、対になる tui_unreadable_windows_test.go
// が DACL でこれを作る。
func denyConfigRead(t *testing.T, path string) {
	t.Helper()
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("deny read on %q: %v", path, err)
	}
}
