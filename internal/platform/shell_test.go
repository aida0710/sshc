package platform

import (
	"slices"
	"testing"
)

// **端末は、それを起こしたものの事情を継がない。**
//
// npm run から起こされた常駐プロセスは npm の設定を環境に持っている。そのまま
// 渡すと、開いたシェルの中で nvm が `npm_config_prefix` を見て「知らない prefix
// だ」と警告する——利用者はそれを選んでいない。実際にそうなっていた。
func TestLoginEnvironmentDropsWhatNpmInjected(t *testing.T) {
	kept := LoginEnvironment([]string{
		"HOME=/Users/someone",
		"npm_config_prefix=/somewhere/desktop",
		"npm_lifecycle_event=start",
		"npm_package_name=sshc-desktop",
		"INIT_CWD=/somewhere",
		"NODE=/somewhere/bin/node",
		// 大文字は人が自分で置いたものである。npm が輸出するのは小文字だけだ。
		"NPM_TOKEN=not-npms-doing",
		"PATH=/usr/bin",
	})

	want := []string{"HOME=/Users/someone", "NPM_TOKEN=not-npms-doing", "PATH=/usr/bin"}
	if !slices.Equal(kept, want) {
		t.Errorf("kept = %q, want %q", kept, want)
	}
}

// Android には /bin/bash も /bin/zsh も居ない。**/bin/sh すら居ない** ——
// Android の sh は /system/bin/sh (mksh) である。ここを間違えると、埋め込み
// ターミナルは「開けるシェルが無い」としか言えなくなる。
func TestShellFallbacksOnAndroidNameTheOnlyShellThatExists(t *testing.T) {
	want := []string{"/system/bin/sh"}
	if got := shellFallbacks("android"); !slices.Equal(got, want) {
		t.Errorf("shellFallbacks(android) = %q, want %q", got, want)
	}
}

// macOS の既定は zsh、それ以外の unix は bash である。**android を足したことで
// この 2 つが変わっていないこと**を、同じ場所で言う。
func TestShellFallbacksKeepTheirExistingOrder(t *testing.T) {
	if got, want := shellFallbacks("darwin"), []string{"/bin/zsh", "/bin/bash", "/bin/sh"}; !slices.Equal(got, want) {
		t.Errorf("shellFallbacks(darwin) = %q, want %q", got, want)
	}
	if got, want := shellFallbacks("linux"), []string{"/bin/bash", "/bin/zsh", "/bin/sh"}; !slices.Equal(got, want) {
		t.Errorf("shellFallbacks(linux) = %q, want %q", got, want)
	}
}
