package validate_test

import (
	"strings"
	"testing"

	"sshc/internal/validate"
)

// 自作のキーに出す文字は、空白だけ・長すぎる・制御文字を含むものを断る。
func TestAKeyBarLabelMustFitOnAKey(t *testing.T) {
	for _, label := range []string{"git st", "|", "ログ", strings.Repeat("あ", validate.MaxKeyBarLabelLength)} {
		if err := validate.KeyBarLabel(label); err != nil {
			t.Errorf("%q was refused: %v", label, err)
		}
	}
	for _, label := range []string{"", "  ", strings.Repeat("a", validate.MaxKeyBarLabelLength+1), "a\tb", "a\x1bb", "\x7f", "\u0085"} {
		if validate.KeyBarLabel(label) == nil {
			t.Errorf("%q was accepted", label)
		}
	}
}

// 自作のキーが送る内容は、Tab と改行だけを制御文字として許す。Enter は「送ったあと Enter を押す」で頼む。
func TestAKeyBarTextMayHoldTabsAndLineFeedsButNoOtherControl(t *testing.T) {
	for _, text := range []string{"git status", "printf 'a\\tb'", "cd ~/src\nls", "a\tb", "日本語"} {
		if err := validate.KeyBarText(text); err != nil {
			t.Errorf("%q was refused: %v", text, err)
		}
	}
	for _, text := range []string{"", "ls\r", "\x03", "\x1b[A", "a\x7f", strings.Repeat("a", validate.MaxKeyBarTextLength+1)} {
		if validate.KeyBarText(text) == nil {
			t.Errorf("%q was accepted", text)
		}
	}
}

func TestOnlyTheListedKeysAreKeyBarKeys(t *testing.T) {
	for _, name := range []string{"Control", "Ctrl+C", "F12", "PageDown"} {
		if !validate.IsKeyBarKey(name) {
			t.Errorf("%q is not a key bar key", name)
		}
	}
	for _, name := range []string{"", "Ctrl", "Esc", "F13", "Ctrl+X"} {
		if validate.IsKeyBarKey(name) {
			t.Errorf("%q is a key bar key", name)
		}
	}
}
