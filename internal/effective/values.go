// Package effective の一部。ssh -G の出力を読む部分だけがここに残っている。
//
// このアプリケーションは ssh -G を回さない。設定の解決は Resolve が行い、
// 何も実行しない。ここにあるのは、その Resolve を実機の OpenSSH と突き合わせる
// 差分テストのための解析器である。一致を確かめるには、相手の言い分を読める
// 必要がある。製品の経路からは呼ばれない。
package effective

import (
	"strings"
)

// Values は、OpenSSH がひとつの alias について報告した実効設定。
// Keywords は出力順を保ち、Entries は、identityfile のように複数回現れうる
// キーワードのすべての値を保つ。
type Values struct {
	Keywords []string
	Entries  map[string][]string
}

// First は keyword の最初の値を返す。なければ空文字列。
func (v Values) First(keyword string) string {
	values := v.Entries[strings.ToLower(keyword)]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// All は keyword のすべての値を出力順で返す。
func (v Values) All(keyword string) []string { return v.Entries[strings.ToLower(keyword)] }

// ParseValues は `ssh -G` の出力を解析する。各行は小文字のキーワード、空白ひとつ、
// そして行の残りで、残りの部分自体が空白を含みうる。
func ParseValues(stdout []byte) Values {
	values := Values{Entries: make(map[string][]string)}
	for _, raw := range strings.Split(string(stdout), "\n") {
		line := strings.TrimRight(raw, "\r")
		if line == "" {
			continue
		}
		keyword, argument, _ := strings.Cut(line, " ")
		keyword = strings.ToLower(keyword)
		if _, seen := values.Entries[keyword]; !seen {
			values.Keywords = append(values.Keywords, keyword)
		}
		values.Entries[keyword] = append(values.Entries[keyword], argument)
	}
	return values
}
