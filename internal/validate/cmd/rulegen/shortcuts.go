package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sshc/internal/validate"
)

// ショートカットの和音は、名前の規則と違って両方向に揃える。画面が記録した和音を
// engine が断れば保存が理由なく失敗し、engine が通す和音を画面が記録できなければ
// その組み合わせは使えない。コーパスの判定は web でも同じでなければならない。

// shortcutChords は、和音について両方の側へ突き合わせてほしい入力である。
//
// 食い違いが実際に出ていたもの（Shift 付きのファンクションキー）を先に置く。
var shortcutChords = []struct{ input, why string }{
	{"Shift+F1", "Shift 付きのファンクションキー。画面は記録し engine は断っていた"},
	{"Ctrl+Shift+F5", "修飾を重ねたファンクションキー"},
	{"F1", "修飾なしのファンクションキー"},
	{"F13", "外付けのキーボードで増やせるファンクションキー"},
	{"F24", "ファンクションキーの上限"},
	{"F25", "ファンクションキーの上限を超える"},
	{"Ctrl+K", "ふつうの和音"},
	{"Ctrl+Alt+Shift+Meta+K", "修飾を全部、決まった順に"},
	{"Shift+Ctrl+K", "修飾の順が違う"},
	{"Ctrl+Ctrl+K", "同じ修飾が重なる"},
	{"Shift+A", "Shift だけでは文字の入力と見分けられない"},
	{"A", "修飾なしの文字"},
	{"Ctrl+f", "キー名は大文字で書く"},
	{"Ctrl+-", "記号のキー"},
	{"Ctrl++", "'+' は区切りと見分けられない"},
	{"Alt+PageDown", "名前のあるキー"},
	{"", "空"},
}

type shortcutCorpus struct {
	Chord []verdict `json:"chord"`
}

func writeShortcutRules(directory string) error {
	if err := os.WriteFile(filepath.Join(directory, "shortcuts.generated.ts"), []byte(shortcutConstants()), 0o644); err != nil {
		return err
	}
	built := shortcutCorpus{}
	for _, item := range shortcutChords {
		built.Chord = append(built.Chord, verdict{
			Input: item.input, Valid: validate.ShortcutChord(item.input) == nil, Why: item.why,
		})
	}
	body, err := json.MarshalIndent(built, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(directory, "shortcutCorpus.generated.json"), append(body, '\n'), 0o644)
}

func shortcutConstants() string {
	var out strings.Builder
	out.WriteString(`// 生成物である。手で編集しない。
//
// 出どころは internal/validate/shortcut.go で、配っているのは cmd/rulegen である。
// 変えるならあちらを変えて make generate を走らせること。
//
// 和音は "Ctrl+Alt+Shift+Meta+キー" の形で、修飾はこの順に 0 個以上並ぶ。
// ファンクションキー以外のキーには Ctrl・Alt・Meta のどれかが要る。

`)
	fmt.Fprintf(&out, "export const shortcutKeyPattern = /%s/;\n", validate.ShortcutKeyPattern)
	fmt.Fprintf(&out, "export const shortcutFunctionKeyPattern = /%s/;\n\n", validate.ShortcutFunctionKeyPattern)
	fmt.Fprintf(&out, "export const shortcutModifiers: readonly string[] = %s;\n", typeScriptList(validate.ShortcutModifiers))
	fmt.Fprintf(&out, "export const shortcutCommandModifiers: readonly string[] = %s;\n\n", typeScriptList(validate.ShortcutCommandModifiers))
	out.WriteString("// ショートカットを割り当てられる操作。並びは設定画面の並びでもある。\n")
	fmt.Fprintf(&out, "export const shortcutActions = %s as const;\n\n", typeScriptList(validate.ShortcutActions))
	fmt.Fprintf(&out, "export const maxShortcutPresets = %d;\n", validate.MaxShortcutPresets)
	fmt.Fprintf(&out, "export const maxShortcutPresetNameLength = %d;\n", validate.MaxShortcutPresetNameLength)
	fmt.Fprintf(&out, "export const maxShortcutKeysPerAction = %d;\n", validate.MaxShortcutKeysPerAction)
	return out.String()
}

func typeScriptList(values []string) string {
	quoted := make([]string, len(values))
	for index, value := range values {
		quoted[index] = fmt.Sprintf("%q", value)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}
