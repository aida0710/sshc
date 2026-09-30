package validate

import (
	"errors"
	"regexp"
	"slices"
	"strings"
)

// ショートカットの和音は、画面が押されたキーから組み立て、engine が保存の前に
// 確かめる。画面が記録した和音を engine が断ると、利用者は理由の分からない保存の
// 失敗を見る。そのため規則をここにひとつ置き、cmd/rulegen が TypeScript へ配る。
//
// 和音は "Ctrl+Alt+Shift+Meta+キー" の形で、修飾はこの順に 0 個以上並ぶ。
const (
	// ShortcutKeyPattern は、和音の最後に置けるキー名である。
	//
	// '+' は区切りと見分けられないので入れない。ファンクションキーは外付けの
	// キーボードで F13 以降も増やせるので、F24 まで許す。
	ShortcutKeyPattern = "^([A-Z0-9]|F([1-9]|1[0-9]|2[0-4])|Arrow(Up|Down|Left|Right)|PageUp|PageDown|Home|End|Insert|Delete|Backspace|Enter|Tab|Escape|Space|[-=,.;/\\[\\]\\\\'`])$"
	// ShortcutFunctionKeyPattern は、修飾なしでも和音にしてよいキーである。
	//
	// ほかのキーは、修飾なしや Shift だけでは文字の入力と見分けられない。
	ShortcutFunctionKeyPattern = `^F([1-9]|1[0-9]|2[0-4])$`
)

// ShortcutModifiers は、和音に書ける修飾の並びである。和音はこの順に書く。
var ShortcutModifiers = []string{"Ctrl", "Alt", "Shift", "Meta"}

// ShortcutCommandModifiers は、ファンクションキー以外のキーと組むときに
// 少なくとも 1 つ要る修飾である。Shift だけでは文字の入力と見分けられない。
var ShortcutCommandModifiers = []string{"Ctrl", "Alt", "Meta"}

// ShortcutActions は、ショートカットを割り当てられる操作である。並びは設定画面の
// 並びでもある。
var ShortcutActions = []string{"palette", "terminalSearch", "copy", "paste", "nextSession", "previousSession", "home", "sftp"}

// ショートカットのプリセットの上限。API（api/openapi.yaml の Metadata.shortcutPresets と
// ShortcutPreset）も同じ値を約束し、internal/acceptance のテストが突き合わせる。
const (
	// MaxShortcutPresets は、同期するプリセットの数の上限である。
	MaxShortcutPresets = 64
	// MaxShortcutPresetNameLength は、プリセット名の文字数の上限である。
	MaxShortcutPresetNameLength = 80
	// MaxShortcutKeysPerAction は、1 つの操作に割り当てられる和音の数の上限である。
	MaxShortcutKeysPerAction = 3
)

// ErrInvalidShortcut は、画面が記録しない形の和音を報告する。
var ErrInvalidShortcut = errors.New("shortcut is not a chord this application records")

var (
	shortcutKey         = regexp.MustCompile(ShortcutKeyPattern)
	shortcutFunctionKey = regexp.MustCompile(ShortcutFunctionKeyPattern)
)

// ShortcutChord は、この和音を保存してよいかを報告する。
func ShortcutChord(chord string) error {
	separator := strings.LastIndex(chord, "+")
	key := chord[separator+1:]
	if !shortcutKey.MatchString(key) {
		return ErrInvalidShortcut
	}
	var modifiers []string
	if separator >= 0 {
		modifiers = strings.Split(chord[:separator], "+")
	}
	if !inShortcutModifierOrder(modifiers) {
		return ErrInvalidShortcut
	}
	commandModifier := func(modifier string) bool { return slices.Contains(ShortcutCommandModifiers, modifier) }
	if !shortcutFunctionKey.MatchString(key) && !slices.ContainsFunc(modifiers, commandModifier) {
		return ErrInvalidShortcut
	}
	return nil
}

// inShortcutModifierOrder は、修飾が ShortcutModifiers の順に、重ならずに並んで
// いるかを報告する。
func inShortcutModifierOrder(modifiers []string) bool {
	next := 0
	for _, modifier := range modifiers {
		for next < len(ShortcutModifiers) && ShortcutModifiers[next] != modifier {
			next++
		}
		if next == len(ShortcutModifiers) {
			return false
		}
		next++
	}
	return true
}
