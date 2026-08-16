//go:build unix

package handoff_test

import (
	"os"
	"path/filepath"
	"testing"

	"sshc/internal/handoff"
)

// writeHandoffFixture は、Read が中身を読むところまで進める handoff を置く。
//
// **中身の不正を試すには、まず入れ物が正しくなければならない。** Read は所有者と
// 権限を先に確かめ、そこで断ったものは復号も解析もしない。その順序は正しいので、
// 解析の失敗を試すフィクスチャは、この OS が私的と認める形で書かれる必要がある。
func writeHandoffFixture(t *testing.T, body []byte) string {
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
