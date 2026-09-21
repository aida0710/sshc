package effective

import "strings"

// ParseValues は外部 package のテスト（差分テスト）へ parseValues を見せる。
var ParseValues = parseValues

// parseValues は `ssh -G` の出力を解析する。テストが OpenSSH の答えと
// 突き合わせるためのもので、製品はこの解決器を権威とし `ssh -G` を読まない。各行は小文字のキーワード、空白ひとつ、
// そして行の残りで、残りの部分自体が空白を含みうる。
func parseValues(stdout []byte) Values {
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

// WinningSource は、keyword について勝った出所を Projection から取り出す。
// 製品は Sources をそのまま表示し、勝者を引く必要がない。
func WinningSource(projection Projection, keyword string) (Source, bool) {
	wanted := strings.ToLower(keyword)
	for _, source := range projection.Sources {
		if source.Winner && strings.ToLower(source.Keyword) == wanted {
			return source, true
		}
	}
	return Source{}, false
}

// IsCumulative は、そのキーワードが積み上がるかを報告する。
func IsCumulative(keyword string) bool { return cumulativeKeywords[strings.ToLower(keyword)] }
