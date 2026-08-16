//go:build !windows

package httpserver

import (
	"testing"

	"sshc/internal/terminal"
)

// ログインシェルとして起こしたことを Unix で伝える手段は、先頭にハイフンの
// 付いた argv[0] だけである。対になる login_shell_windows_test.go が向こうの
// 手段を確かめる——Windows にこの合図を読むシェルは居ない。
func assertOpenedAsALoginShell(t *testing.T, command terminal.Command) {
	t.Helper()
	if command.Argv0 == "" || command.Argv0[0] != '-' {
		t.Fatalf("command = %#v, want a login argv[0]", command)
	}
}
