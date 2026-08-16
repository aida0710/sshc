//go:build !windows

// Package acltest builds the private-state fixtures that the tests need and
// that an ordinary write cannot produce.
package acltest

import (
	"os"
	"path/filepath"
	"testing"
)

// WritePrivateFile places a file that the private-state readers will accept.
//
// **中身を試すには、まず入れ物が正しくなければならない。** private state の
// 読み口は所有者と権限を先に確かめ、そこで断ったものは解析しない。
func WritePrivateFile(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}
