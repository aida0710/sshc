package effective

import (
	"maps"
	"slices"
	"strings"

	"sshc/internal/config"
)

// restOfLineKeywords は、OpenSSH が引数に分けず、キーワードの後ろの行の残りを
// そのまま値にするキーワードである。readconf.c の parse_command に当たる。
//
// 行の残りには引用符も行末のコメントも含まれる。`ssh -G` もそのまま出す。
// コマンドとして解釈するのはシェルであり、ssh_config の字句解析ではない。
var restOfLineKeywords = map[string]bool{
	"proxycommand": true, "remotecommand": true, "localcommand": true, "knownhostscommand": true,
}

// TakesRestOfLine は、keyword の値が行の残りそのものかを報告する。
//
// 画面で編集するときも、値を引数に分けず、引用し直さずに書く。引用し直すと、
// 一重引用符が二重引用符に変わり、コマンドを読むシェルが $1 を展開するようになる。
func TakesRestOfLine(keyword string) bool {
	return restOfLineKeywords[strings.ToLower(keyword)]
}

// RestOfLineKeywords は、TakesRestOfLine が真になるキーワードを小文字の辞書順で返す。
// ブラウザへ同じ規則を配るためにある。
func RestOfLineKeywords() []string {
	return slices.Sorted(maps.Keys(restOfLineKeywords))
}

// argumentListKeywords は、OpenSSH が引数のひとつずつを別の値として持つキーワード
// である。`SetEnv X="a b" ONE=1` の値は "X=a b" と "ONE=1" の二つで、空白で
// つなぐと境界が失われる。
var argumentListKeywords = map[string]bool{
	"setenv": true, "sendenv": true, "userknownhostsfile": true, "globalknownhostsfile": true,
}

// directiveValues は、ディレクティブ一行が OpenSSH に渡す値を返す。値の無い行は nil。
//
// 解決の値（Values）、採用した行の値（Accepted）、出所の表示（Source）は、
// どれもこの関数から取る。同じ行から別々の値を作ると、画面の表示と接続に使う
// 値が食い違う。
func directiveValues(line config.Line) []string {
	keyword := strings.ToLower(line.Keyword)
	if restOfLineKeywords[keyword] {
		return nonEmpty(ArgumentText(line))
	}
	values := line.Values()
	if argumentListKeywords[keyword] {
		return values
	}
	return nonEmpty(strings.Join(values, " "))
}

func nonEmpty(value string) []string {
	if value == "" {
		return nil
	}
	return []string{value}
}

// ArgumentText は、ディレクティブの引数部分を、インデント・キーワード・区切り・
// 行末の空白を除いて、書かれたとおりに返す。
func ArgumentText(line config.Line) string {
	var builder strings.Builder
	for _, argument := range line.Arguments {
		builder.WriteString(argument.Lead)
		builder.WriteString(argument.Raw)
	}
	return strings.TrimSpace(builder.String())
}
