package main

import (
	"os"
	"runtime"
	"testing"
)

// shortSocketDirectory は、Unix ソケットを置ける短いパスの一時ディレクトリを返す。
//
// t.TempDir は macOS では /var/folders/... の下になり、ソケットのパスの上限
// （104 バイト）を超えることがある。
func shortSocketDirectory(t *testing.T) string {
	t.Helper()
	base := ""
	if runtime.GOOS != "windows" {
		base = "/tmp"
	}
	directory, err := os.MkdirTemp(base, "sshc-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	return directory
}
