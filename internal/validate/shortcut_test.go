package validate_test

import (
	"testing"

	"sshc/internal/validate"
)

// ファンクションキーは、修飾なしでも、Shift を含む修飾付きでも和音になる。
// 外付けのキーボードで増やせる F13 以降も同じ。画面は Shift+F1 を記録するのに
// engine が断っていた。
func TestAFunctionKeyIsAChordWithOrWithoutModifiers(t *testing.T) {
	for _, chord := range []string{"F1", "F12", "F13", "F24", "Shift+F1", "Ctrl+Shift+F5", "Alt+F13", "Ctrl+Alt+Shift+Meta+F24"} {
		if err := validate.ShortcutChord(chord); err != nil {
			t.Errorf("%q was refused: %v", chord, err)
		}
	}
}

// ファンクションキー以外は、Ctrl・Alt・Meta のどれかが要る。Shift だけでは文字の
// 入力と見分けられない。
func TestAnOrdinaryKeyNeedsCtrlAltOrMeta(t *testing.T) {
	for _, chord := range []string{"Ctrl+K", "Alt+PageDown", "Meta+C", "Ctrl+Shift+V", "Shift+Meta+K", "Ctrl+-"} {
		if err := validate.ShortcutChord(chord); err != nil {
			t.Errorf("%q was refused: %v", chord, err)
		}
	}
	for _, chord := range []string{"A", "Shift+A", "Enter", "Shift+Enter"} {
		if validate.ShortcutChord(chord) == nil {
			t.Errorf("%q was accepted without Ctrl, Alt or Meta", chord)
		}
	}
}

// 画面が組み立てない綴りは断る。修飾の順と重なり、キー名の大小、存在しない
// ファンクションキー、区切りと見分けられない '+'。
func TestAChordTheScreenDoesNotBuildIsRefused(t *testing.T) {
	for _, chord := range []string{"", "+", "Ctrl+", "Ctrl++", "+K", "Shift+Ctrl+K", "Ctrl+Ctrl+K", "Ctrl+Alt+Alt+X", "Ctrl+f", "Control+K", "F0", "F25", "Shift+F25"} {
		if validate.ShortcutChord(chord) == nil {
			t.Errorf("%q was accepted", chord)
		}
	}
}
