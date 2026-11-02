package validate

import (
	"errors"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// スマホのキーバーに置くキーは、画面が並べ、engine が保存の前に確かめる。画面が並べたキーを
// engine が断ると、利用者は理由の分からない保存の失敗を見る。そのため名前の一覧と上限をここに
// ひとつ置き、cmd/rulegen が TypeScript へ配る。

// KeyBarKeys は、キーバーに置ける用意したキーの名前である。名前は KeyboardEvent.key に合わせ、
// Ctrl との組み合わせはショートカットの和音と同じ "Ctrl+C" の形にする。DirectInput はキーではなく、
// 打った文字の行き先（下の入力欄か、ターミナルへそのまま）を切り替える。
var KeyBarKeys = []string{
	"DirectInput", "Control", "Alt", "Escape", "Tab", "Enter",
	"Backspace", "Delete", "Insert",
	"ArrowLeft", "ArrowUp", "ArrowDown", "ArrowRight", "Home", "End", "PageUp", "PageDown",
	"Ctrl+C", "Ctrl+D", "Ctrl+Z", "Ctrl+L", "Ctrl+R", "Ctrl+A", "Ctrl+E",
	"F1", "F2", "F3", "F4", "F5", "F6", "F7", "F8", "F9", "F10", "F11", "F12",
}

// キーバーのプリセットの上限。API（api/openapi.yaml の Metadata.keyBarPresets・KeyBarPreset・
// KeyBarKey）も同じ値を約束し、internal/acceptance のテストが突き合わせる。
const (
	// MaxKeyBarPresets は、同期するプリセットの数の上限である。
	MaxKeyBarPresets = 64
	// MaxKeyBarPresetNameLength は、プリセット名の文字数の上限である。
	MaxKeyBarPresetNameLength = 80
	// MaxKeyBarKeys は、いつも出す段のキーの数の上限である。収まらない分は横に送って見る。
	MaxKeyBarKeys = 16
	// MaxKeyBarMoreKeys は、「…」の中のキーの数の上限である。
	MaxKeyBarMoreKeys = 48
	// MaxKeyBarLabelLength は、自作のキーに出す文字の数の上限である。キーの幅に収まる長さにする。
	MaxKeyBarLabelLength = 8
	// MaxKeyBarTextLength は、自作のキーが送る内容の文字数の上限である。
	MaxKeyBarTextLength = 1000
)

var (
	// ErrInvalidKeyBarLabel は、自作のキーに出せない文字を報告する。
	ErrInvalidKeyBarLabel = errors.New("key bar label is empty, too long or has a control character")
	// ErrInvalidKeyBarText は、自作のキーが送れない内容を報告する。
	ErrInvalidKeyBarText = errors.New("key bar text is empty, too long or has a control character other than tab and line feed")
)

// IsKeyBarKey は、name が用意したキーの名前かを返す。
func IsKeyBarKey(name string) bool {
	return slices.Contains(KeyBarKeys, name)
}

// KeyBarLabelForbiddenPattern は、自作のキーに出す文字に使えない文字（C0・DEL・C1 の制御文字）である。
const KeyBarLabelForbiddenPattern = `[\x00-\x1f\x7f-\x9f]`

// KeyBarTextForbiddenPattern は、自作のキーが送る内容に使えない文字である。Tab と改行のほかの制御文字を
// 断る。同期したプリセットのキーが Esc や Ctrl の文字を黙って送ると、押した人が何を送ったか分からない
// ため。Enter は「送ったあと Enter を押す」で頼む。
const KeyBarTextForbiddenPattern = `[\x00-\x08\x0b-\x1f\x7f-\x9f]`

var (
	keyBarLabelForbidden = regexp.MustCompile(KeyBarLabelForbiddenPattern)
	keyBarTextForbidden  = regexp.MustCompile(KeyBarTextForbiddenPattern)
)

// KeyBarLabel は、自作のキーに出す文字を確かめる。空白だけ・長すぎる・制御文字を含むものを断る。
func KeyBarLabel(label string) error {
	if strings.TrimSpace(label) == "" || !utf8.ValidString(label) || utf8.RuneCountInString(label) > MaxKeyBarLabelLength || keyBarLabelForbidden.MatchString(label) {
		return ErrInvalidKeyBarLabel
	}
	return nil
}

// KeyBarText は、自作のキーが送る内容を確かめる。
func KeyBarText(text string) error {
	if text == "" || !utf8.ValidString(text) || utf8.RuneCountInString(text) > MaxKeyBarTextLength || keyBarTextForbidden.MatchString(text) {
		return ErrInvalidKeyBarText
	}
	return nil
}
