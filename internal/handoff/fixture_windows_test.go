//go:build windows

package handoff_test

import (
	"path/filepath"
	"testing"

	"sshc/internal/handoff"
	"sshc/internal/platform/windowsacl"
)

// writeHandoffFixture は、Read が中身を読むところまで進める handoff を置く。
//
// **中身の不正を試すには、まず入れ物が正しくなければならない。** Read は所有者と
// 保護 DACL を先に確かめ、そこで断ったものは解析しない。その順序は正しいので、
// os.WriteFile で置いた素のファイルでは解析にたどり着けない。ここでは本番と
// 同じ経路で作り、そのうえで中身だけを壊す。
func writeHandoffFixture(t *testing.T, body []byte) string {
	t.Helper()
	directory := filepath.Join(t.TempDir(), "state")
	if err := windowsacl.EnsureDirectory(directory); err != nil {
		t.Fatal(err)
	}
	file, err := windowsacl.OpenOrCreateFile(filepath.Join(directory, handoff.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(body); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return directory
}
