package application

import (
	"strings"

	"sshc/internal/config"
	"sshc/internal/effective"
)

// formValues は、フォームで編集する行の値を返す。
//
// ProxyCommand などの行の残りを値にするキーワードは、書かれたとおりの行の残りを
// ひとつの値にする。引用を外した引数に分けると、画面がそれを二重引用符で書き直し、
// 保存したときにシェルが読む文字列が変わる。
func formValues(line config.Line) []string {
	if !effective.TakesRestOfLine(line.Keyword) {
		return line.Values()
	}
	text := effective.ArgumentText(line)
	if text == "" {
		return nil
	}
	return []string{text}
}

// rebuildRestOfLine は、行の残りを値にするキーワードの行を、values を引用せずに
// 書き直す。インデント・区切り・改行は元の行のまま残す。
func rebuildRestOfLine(line config.Line, keyword string, values []string) (config.Line, error) {
	arguments, err := restOfLineArguments(keyword, values)
	if err != nil {
		return config.Line{}, err
	}
	if len(arguments) > 0 && len(line.Arguments) > 0 {
		arguments[0].Lead = line.Arguments[0].Lead
	}
	rebuilt := line
	rebuilt.Keyword = keyword
	if rebuilt.Separator == "" {
		rebuilt.Separator = " "
	}
	rebuilt.Arguments = arguments
	return rebuilt, nil
}

// restOfLineArguments は、行の残りとして書く値を、ファイルから読み直したときと同じ
// 引数の並びにする。values が複数あれば空白ひとつでつなぐ。
//
// 前後の空白は OpenSSH も読み飛ばすので落とす。書けないとして断るのは、行を分ける
// 改行と NUL、区切りとして読み飛ばされる先頭の '='、OpenSSH が「invalid quotes」と
// して設定ごと断る閉じない引用である。
func restOfLineArguments(keyword string, values []string) ([]config.Argument, error) {
	text := strings.Trim(strings.Join(values, " "), " \t")
	if text == "" {
		return nil, nil
	}
	if strings.ContainsAny(text, "\n\r\x00") {
		return nil, ErrUnquotableValue
	}
	parsed := config.Parse([]byte(keyword + " " + text)).Lines
	if len(parsed) != 1 || parsed[0].Kind != config.LineDirective || parsed[0].Separator != " " {
		return nil, ErrUnquotableValue
	}
	arguments := parsed[0].Arguments
	arguments[0].Lead = ""
	return arguments, nil
}
