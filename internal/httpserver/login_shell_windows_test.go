//go:build windows

package httpserver

import (
	"testing"

	"sshc/internal/terminal"
)

// Windows でログインシェルとして起動する手段は、引数だけである。ハイフン付きの
// argv[0] を渡せば、それは表記を間違えた引数にしか見えない。
//
// このフィクスチャのシェルは PowerShell ではないので、黙らせる著作権表示も
// 無い。ここで表明するのは、Unix の合図が作られないことである。
func assertOpenedAsALoginShell(t *testing.T, command terminal.Command) {
	t.Helper()
	if command.Argv0 != "" {
		t.Fatalf("command = %#v, want no unix login argv[0] on Windows", command)
	}
}
