package platform

import (
	"slices"
	"strings"
	"testing"
)

// 端末は、それを起動したものの事情を継がない。
//
// npm run から起動された常駐プロセスは npm の設定を環境に持っている。そのまま
// 渡すと、開いたシェルの中で nvm が `npm_config_prefix` を見て「知らない prefix
// だ」と警告する。利用者はそれを選んでいない。実際にそうなっていた。
func TestLoginEnvironmentDropsWhatNpmInjected(t *testing.T) {
	kept := LoginEnvironment([]string{
		"HOME=/Users/someone",
		"TERM=dumb",
		"npm_config_prefix=/somewhere/desktop",
		"npm_lifecycle_event=start",
		"npm_package_name=sshc-desktop",
		"INIT_CWD=/somewhere",
		"NODE=/somewhere/bin/node",
		// 大文字はユーザーが自分で置いたものである。npm が輸出するのは小文字だけだ。
		"NPM_TOKEN=not-npms-doing",
		"PATH=/usr/bin",
	})

	want := []string{"HOME=/Users/someone", "NPM_TOKEN=not-npms-doing", "PATH=/usr/bin", "TERM=xterm-256color"}
	if !slices.Equal(kept, want) {
		t.Errorf("kept = %q, want %q", kept, want)
	}
}

func TestLoginEnvironmentAlwaysDescribesTheEmbeddedTerminal(t *testing.T) {
	for _, environment := range [][]string{
		nil,
		{"HOME=/Users/someone"},
		{"TERM=xterm-kitty", "HOME=/Users/someone"},
		{"Term=dumb", "HOME=/Users/someone"},
	} {
		kept := LoginEnvironment(environment)
		if got := kept[len(kept)-1]; got != "TERM="+LocalTerminalType {
			t.Errorf("LoginEnvironment(%q) terminal = %q", environment, got)
		}
		count := 0
		for _, entry := range kept {
			name, _, _ := strings.Cut(entry, "=")
			if strings.EqualFold(name, "TERM") {
				count++
			}
		}
		if count != 1 {
			t.Errorf("LoginEnvironment(%q) has %d TERM entries: %q", environment, count, kept)
		}
	}
}
