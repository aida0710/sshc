//go:build unix

package main

import (
	"os"
	"path/filepath"
	"testing"

	"sshc/internal/handoff"
)

// writeRawHandoff は、読み手が中身を解析するところまで進める handoff を置く。
//
// **中身の不正を試すには、まず入れ物が正しくなければならない。** 読み口は所有者と
// 権限を先に確かめ、そこで断ったものは解析しない。
func writeRawHandoff(t *testing.T, body []byte) string {
	t.Helper()
	directory := filepath.Join(t.TempDir(), "state")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, handoff.FileName), body, 0o600); err != nil {
		t.Fatal(err)
	}
	return directory
}
