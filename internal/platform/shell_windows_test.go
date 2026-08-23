//go:build windows

package platform

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// このファイルは Windows の runner でしか走らない。どこから選ぶかという方針
// そのものは internal/platform/windows の検査がどの OS でも走らせるので、ここで
// 確かめるのは、この機械が本当に返すかどうかだけである。Windows 自身に尋ねた
// 在り処に、実在する PowerShell があること。

// 環境を一つも渡されなくても、シェルは見つからなければならない。
//
// ログイン項目や launcher から起動された常駐プロセスは、環境を持っていない
// ことがある。Unix が /bin の絶対パスで返すのと同じ場面で、Windows が
// 「シェルが無い」と応答すれば、ローカル端末は一本も開かない。
func TestTheLoginShellNeedsNoEnvironmentOnWindows(t *testing.T) {
	shell, err := LoginShell(nil)
	if err != nil {
		t.Fatalf("LoginShell(nil) = %v", err)
	}
	info, err := os.Stat(shell)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("LoginShell(nil) = %q, which is not a program: %v", shell, err)
	}
	if name := strings.ToLower(filepath.Base(shell)); name != "pwsh.exe" && name != "powershell.exe" {
		t.Errorf("LoginShell(nil) = %q, want the bundled PowerShell", shell)
	}
	if argv0 := LoginArgv0(shell); argv0 != "" {
		t.Errorf("LoginArgv0() = %q, want none on Windows", argv0)
	}
	if arguments := LoginArguments(shell); !slices.Equal(arguments, []string{"-NoLogo"}) {
		t.Errorf("LoginArguments() = %q, want -NoLogo", arguments)
	}
}
