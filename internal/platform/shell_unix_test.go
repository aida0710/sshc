//go:build !windows

package platform

import (
	"runtime"
	"slices"
	"testing"
)

// このファイルは、Unix のシェル探索でしか言えないことを持つ。/bin/sh はどの
// Unix にもあり、Windows には無い。実行ビットも向こうには無く、ハイフン付きの
// argv[0] を読むシェルも居ない。同じ検査を両方で走らせると、落ちるだけでなく
// 「ここに同じ規則がある」という誤りが残る。対になる shell_windows_test.go が
// 向こうの規則を確かめる。

func TestTheLoginShellIsTheOneTheUserChose(t *testing.T) {
	shell, err := LoginShell(func(name string) (string, bool) {
		return "/bin/sh", name == "SHELL"
	})
	if err != nil {
		t.Fatalf("LoginShell() = %v", err)
	}
	if shell != "/bin/sh" {
		t.Errorf("LoginShell() = %q, want /bin/sh", shell)
	}
}

// PATH 経由で解決される表記は受け取らない。受け取れば、端末に渡るのは
// このアプリケーションが選んだのではないプログラムになる。ディレクトリと、
// 実行できないファイルも同じく候補ではない。
func TestTheLoginShellFallsBackWhenTheChoiceIsNotAShell(t *testing.T) {
	for name, value := range map[string]string{
		"relative":    "sh",
		"directory":   "/bin",
		"not there":   "/bin/no-such-shell",
		"not a shell": "/etc/hosts",
		"unset":       "",
	} {
		t.Run(name, func(t *testing.T) {
			shell, err := LoginShell(func(string) (string, bool) { return value, value != "" })
			if err != nil {
				t.Fatalf("LoginShell() = %v", err)
			}
			if !slices.Contains(shellFallbacks(runtime.GOOS), shell) {
				t.Errorf("LoginShell() = %q, want one of %q", shell, shellFallbacks(runtime.GOOS))
			}
		})
	}
}

func TestTheLoginShellFindsAShellWithoutAnyEnvironment(t *testing.T) {
	shell, err := LoginShell(nil)
	if err != nil {
		t.Fatalf("LoginShell() = %v", err)
	}
	if !slices.Contains(shellFallbacks(runtime.GOOS), shell) {
		t.Errorf("LoginShell() = %q, want one of %q", shell, shellFallbacks(runtime.GOOS))
	}
}

// 先頭のハイフンが、そのシェルにログインシェルとしての起動を伝える唯一の手段で
// ある。Unix はそれで足りるので、渡す引数は無い。
func TestTheLoginArgv0CarriesTheHyphen(t *testing.T) {
	if got := LoginArgv0("/bin/zsh"); got != "-zsh" {
		t.Errorf("LoginArgv0() = %q, want -zsh", got)
	}
	if got := LoginArguments("/bin/zsh"); got != nil {
		t.Errorf("LoginArguments() = %q, want none", got)
	}
}

// Android には /bin/bash も /bin/zsh も居ない。/bin/sh すら居ない。
// Android の sh は /system/bin/sh (mksh) である。ここを間違えると、埋め込み
// ターミナルは「開けるシェルが無い」としか言えなくなる。
func TestShellFallbacksOnAndroidNameTheOnlyShellThatExists(t *testing.T) {
	want := []string{"/system/bin/sh"}
	if got := shellFallbacks("android"); !slices.Equal(got, want) {
		t.Errorf("shellFallbacks(android) = %q, want %q", got, want)
	}
}

// iOS では、どのシェルも開けない。サンドボックスが fork も exec も禁じている
// ので、置いてあるかどうか以前の話である。
//
// 候補を並べると、埋め込みターミナルは存在しない道を毎回探し、開けなかった理由を
// 「見つからない」と言う。本当の理由は「この OS では開けない」であり、それは
// 探して分かることではない。
func TestShellFallbacksOnIOSNameNothingAtAll(t *testing.T) {
	if got := shellFallbacks("ios"); len(got) != 0 {
		t.Errorf("shellFallbacks(ios) = %q, want nothing; iOS cannot start a process", got)
	}
}

// macOS の既定は zsh、それ以外の unix は bash である。android を足したことで
// この 2 つが変わっていないことを、同じ場所で言う。
func TestShellFallbacksKeepTheirExistingOrder(t *testing.T) {
	if got, want := shellFallbacks("darwin"), []string{"/bin/zsh", "/bin/bash", "/bin/sh"}; !slices.Equal(got, want) {
		t.Errorf("shellFallbacks(darwin) = %q, want %q", got, want)
	}
	if got, want := shellFallbacks("linux"), []string{"/bin/bash", "/bin/zsh", "/bin/sh"}; !slices.Equal(got, want) {
		t.Errorf("shellFallbacks(linux) = %q, want %q", got, want)
	}
}
