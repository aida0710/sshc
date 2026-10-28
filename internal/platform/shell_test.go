package platform

import (
	"slices"
	"strings"
	"testing"

	"sshc/internal/terminal"
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
	}, "v0.58.0")

	want := []string{
		"HOME=/Users/someone", "NPM_TOKEN=not-npms-doing", "PATH=/usr/bin",
		"TERM=xterm-256color", "COLORTERM=truecolor", "TERM_PROGRAM=sshc", "TERM_PROGRAM_VERSION=0.58.0",
	}
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
		kept := LoginEnvironment(environment, "dev")
		var terms []string
		for _, entry := range kept {
			name, _, _ := strings.Cut(entry, "=")
			if strings.EqualFold(name, "TERM") {
				terms = append(terms, entry)
			}
		}
		if !slices.Equal(terms, []string{"TERM=" + terminal.DefaultTerminalType}) {
			t.Errorf("LoginEnvironment(%q) TERM entries = %q", environment, terms)
		}
	}
}

// sshcエンジンを Ghostty や Windows Terminal の中で起動すると、そのターミナルが自分のために
// 置いた変数が、sshc のターミナルの中のシェルへそのまま継がれていた。
func TestLoginEnvironmentReplacesWhatTheLaunchingTerminalSet(t *testing.T) {
	kept := LoginEnvironment([]string{
		"HOME=/Users/someone",
		"TERM_PROGRAM=ghostty",
		"TERM_PROGRAM_VERSION=1.4.0",
		"COLORTERM=24bit",
		"GHOSTTY_RESOURCES_DIR=/Applications/Ghostty.app/Contents/Resources/ghostty",
		"GHOSTTY_SHELL_FEATURES=cursor,title",
		"KITTY_WINDOW_ID=3",
		"Wt_Session=0b1a2c3d",
		"VSCODE_INJECTION=1",
		"LC_TERMINAL=iTerm2",
		"TERMINFO=/Applications/Ghostty.app/Contents/Resources/terminfo",
		"PATH=/usr/bin",
	}, "v0.58.0")

	want := []string{
		"HOME=/Users/someone", "PATH=/usr/bin",
		"TERM=xterm-256color", "COLORTERM=truecolor", "TERM_PROGRAM=sshc", "TERM_PROGRAM_VERSION=0.58.0",
	}
	if !slices.Equal(kept, want) {
		t.Errorf("kept = %q, want %q", kept, want)
	}
}
