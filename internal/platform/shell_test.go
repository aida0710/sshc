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
