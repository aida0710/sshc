//go:build !windows

// Package acltest は通常の書き込みでは作れない非公開状態のテスト fixture を作る。
package acltest

import (
	"os"
	"path/filepath"
	"testing"
)

// WritePrivateFile は非公開状態の reader が受け付けるファイルを配置する。
//
// 中身を試すには、まず入れ物が正しくなければならない。private state の
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
