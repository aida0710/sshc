//go:build windows

package main

import (
	"path/filepath"
	"testing"

	"sshc/internal/platform/windowsacl"
)

// writePrivateStateFile は、読み手が中身を解析するところまで進める private state を
// 置く。
//
// **中身を試すには、まず入れ物が正しくなければならない。** private state の読み口は
// 所有者と保護 DACL を先に確かめ、そこで断ったものは解析しない。素の os.WriteFile
// ではそこへ届かないので、本番と同じ経路で作る。
func writePrivateStateFile(t *testing.T, directory, name string, body []byte) {
	t.Helper()
	if err := windowsacl.EnsureDirectory(directory); err != nil {
		t.Fatal(err)
	}
	file, err := windowsacl.OpenOrCreateFile(filepath.Join(directory, name))
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
}
