//go:build windows

package main

import (
	"path/filepath"
	"testing"

	"sshc/internal/handoff"
	"sshc/internal/platform/windowsacl"
)

// writeRawHandoff は、読み手が中身を解析するところまで進める handoff を置く。
//
// **中身の不正を試すには、まず入れ物が正しくなければならない。** 読み口は所有者と
// 保護 DACL を先に確かめ、そこで断ったものは解析しない。素の os.WriteFile では
// そこへ届かないので、本番と同じ経路で作り、中身だけを壊す。
func writeRawHandoff(t *testing.T, body []byte) string {
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
