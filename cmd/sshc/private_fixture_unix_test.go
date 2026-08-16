//go:build unix

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// writePrivateStateFile は、読み手が中身を解析するところまで進める private state を
// 置く。
//
// **中身を試すには、まず入れ物が正しくなければならない。** private state の読み口は
// 所有者と権限を先に確かめ、そこで断ったものは解析しない。
func writePrivateStateFile(t *testing.T, directory, name string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, name), body, 0o600); err != nil {
		t.Fatal(err)
	}
}
