package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sshc/internal/validate"
)

func writeKeyBarRules(directory string) error {
	return os.WriteFile(filepath.Join(directory, "keyBar.generated.ts"), []byte(keyBarConstants()), 0o644)
}

func keyBarConstants() string {
	var out strings.Builder
	out.WriteString(`// 生成物である。手で編集しない。
//
// 出どころは internal/validate/keybar.go で、配っているのは cmd/rulegen である。
// 変えるならあちらを変えて make generate を走らせること。

`)
	out.WriteString("// キーバーに置ける用意したキーの名前。\n")
	fmt.Fprintf(&out, "export const keyBarKeys = %s as const;\n\n", typeScriptList(validate.KeyBarKeys))
	out.WriteString("// 自作のキーに出す文字と、送る内容に使えない文字。\n")
	fmt.Fprintf(&out, "export const keyBarLabelForbiddenPattern = /%s/u;\n", validate.KeyBarLabelForbiddenPattern)
	fmt.Fprintf(&out, "export const keyBarTextForbiddenPattern = /%s/u;\n\n", validate.KeyBarTextForbiddenPattern)
	fmt.Fprintf(&out, "export const maxKeyBarPresets = %d;\n", validate.MaxKeyBarPresets)
	fmt.Fprintf(&out, "export const maxKeyBarPresetNameLength = %d;\n", validate.MaxKeyBarPresetNameLength)
	fmt.Fprintf(&out, "export const maxKeyBarKeys = %d;\n", validate.MaxKeyBarKeys)
	fmt.Fprintf(&out, "export const maxKeyBarMoreKeys = %d;\n", validate.MaxKeyBarMoreKeys)
	fmt.Fprintf(&out, "export const maxKeyBarLabelLength = %d;\n", validate.MaxKeyBarLabelLength)
	fmt.Fprintf(&out, "export const maxKeyBarTextLength = %d;\n", validate.MaxKeyBarTextLength)
	return out.String()
}
