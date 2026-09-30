package main

import (
	"testing"
	"unicode"

	"sshc/cmd/sshc/internal/clispec"
)

// CLI が書く文（help、質問、状態、失敗の文）は英語で書く（docs/writing-style.md）。
// 画面と pages の日本語、sshcエンジンのターミナルの接続ログはこの対象ではない。

func TestEveryHelpTopicIsWrittenInEnglish(t *testing.T) {
	topics := map[string]string{"sshc help": clispec.GlobalHelp}
	for _, command := range clispec.Commands {
		topics["sshc help "+command.Name] = command.Help
		for _, action := range command.Actions {
			topics["sshc help "+command.Name+" "+action.Name] = action.Help
		}
	}
	for topic, help := range topics {
		if containsJapanese(help) {
			t.Errorf("%s is not English: %q", topic, help)
		}
	}
}

// containsJapanese は、文に日本語（ひらがな、カタカナ、漢字）が混ざっているかを返す。
func containsJapanese(text string) bool {
	for _, character := range text {
		if unicode.In(character, unicode.Hiragana, unicode.Katakana, unicode.Han) {
			return true
		}
	}
	return false
}
