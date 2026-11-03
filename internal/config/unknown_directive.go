package config

import (
	"runtime"
	"strings"

	"sshc/internal/sshmatch"
)

// openSSHKeywords は openSSHKeywordNames（openssh_keywords.gen.go）を引けるようにしたもの。
var openSSHKeywords = func() map[string]bool {
	known := make(map[string]bool, len(openSSHKeywordNames))
	for _, name := range openSSHKeywordNames {
		known[name] = true
	}
	return known
}()

// appleOpenSSHKeywords は、macOS に付いてくる ssh（Apple が手を入れた OpenSSH）だけが
// 受け付けるキーワードである。upstream の OpenSSH はこれを知らないと言って止まる。
// macOS で `UseKeychain yes` を知らせると、そのマシンの ssh が読める行を直させることになる。
var appleOpenSSHKeywords = map[string]bool{"usekeychain": true}

// openSSHKnows は、goos の上の OpenSSH の ssh がキーワードを受け付けるかを返す。
// OpenSSH は ASCII の英字だけを小文字にしてから表と比べる（readconf.c の lowercase）。
func openSSHKnows(keyword, goos string) bool {
	lowered := sshmatch.LowerASCII(keyword)
	return openSSHKeywords[lowered] || (goos == "darwin" && appleOpenSSHKeywords[lowered])
}

// checkKeyword は、ディレクティブの行ひとつを読み込み順に受け取り、OpenSSH が知らない
// キーワードなら知らせる。IgnoreUnknown の行は、その後ろの行のために覚えておく。
//
// OpenSSH は、知らないキーワードの行がどこにあっても止まる。一致しない Host や Match の
// 中でも、一致しないブロックから Include したファイルの中でも同じである。
func (g *Graph) checkKeyword(filePath string, lineNumber int, line Line, unconditional bool) {
	if sshmatch.LowerASCII(line.Keyword) == "ignoreunknown" {
		g.ignoreUnknown.read(line, unconditional)
		return
	}
	if openSSHKnows(line.Keyword, runtime.GOOS) || g.ignoreUnknown.ignoresEverywhere(line.Keyword) {
		return
	}
	g.diagnose(SeverityWarning, DiagnosticUnknownDirective, filePath, lineNumber, line.Keyword)
}

// ignoreUnknownState は、読み込み順でここまでに読んだ IgnoreUnknown の値である。
//
// OpenSSH が使うのは、効いているブロックで最初に読んだ IgnoreUnknown の値ひとつだけである
// （readconf.c の parse_string）。どのブロックが効くかは接続先で変わるので、どの接続先でも
// 止まらないと言えるときだけ、知らないキーワードを知らせない。
type ignoreUnknownState struct {
	// conditional は、どの接続先でも読まれるとは言えない位置の IgnoreUnknown の値である。
	// その位置が効く接続先では、これが最初の値になる。
	conditional []string
	// unconditional は、どの接続先でも読まれる位置の最初の IgnoreUnknown の値である。
	// これを読んだあとは、どの接続先でも値が決まっているので、後ろの IgnoreUnknown は効かない。
	unconditional string
	// settled は、unconditional を読んだかである。
	settled bool
}

func (s *ignoreUnknownState) read(line Line, unconditional bool) {
	values := line.Values()
	if s.settled || len(values) == 0 {
		return
	}
	// 値は最初の引数だけを使う。2つ目からは OpenSSH が余分な引数として断る。
	if unconditional {
		s.unconditional, s.settled = values[0], true
		return
	}
	s.conditional = append(s.conditional, values[0])
}

// ignoresEverywhere は、どの接続先でも、そのとき効いている IgnoreUnknown の値がキーワードに
// 一致するかを返す。どの接続先でも読まれる IgnoreUnknown がまだ無ければ、どれも効かない
// 接続先があるので偽である。
func (s *ignoreUnknownState) ignoresEverywhere(keyword string) bool {
	if !s.settled {
		return false
	}
	lowered := sshmatch.LowerASCII(keyword)
	if !ignoreUnknownMatches(s.unconditional, lowered) {
		return false
	}
	for _, value := range s.conditional {
		if !ignoreUnknownMatches(value, lowered) {
			return false
		}
	}
	return true
}

// ignoreUnknownMatches は、IgnoreUnknown の値ひとつが小文字のキーワードに一致するかを返す。
// OpenSSH は match_pattern_list でカンマ区切りのパターンと比べ、'!' の付いたパターンに
// 当たれば一致しないとする。
func ignoreUnknownMatches(value, loweredKeyword string) bool {
	return sshmatch.PatternList(strings.Split(value, ","), loweredKeyword, sshmatch.IgnoreCase)
}

// unconditionalLines は、ファイルの行ごとに、どの接続先でも読まれる位置にあるかを返す。
// inherited は、このファイルを取り込んだ Include 行がそうであったかで、入口のファイルでは
// 真である。最初の Host / Match より前の行は、取り込み元の状態を引き継ぐ。
//
// 取り込み元が一致しない接続先では、OpenSSH は取り込んだファイルの Host / Match をどれも
// 一致させない（SSHCONF_NEVERMATCH）。だから inherited が偽なら、`Host *` の中も
// どの接続先でも読まれるとは言えない。
func unconditionalLines(file *File, inherited bool) []bool {
	lines := make([]bool, len(file.Lines))
	for _, block := range file.Blocks() {
		everywhere := inherited && blockAppliesEverywhere(block)
		for index := block.Start; index < block.End; index++ {
			lines[index] = everywhere
		}
	}
	return lines
}

// blockAppliesEverywhere は、ブロックがどの接続先にも効くかを返す。`Host *`（否定の
// パターンを持たない）と `Match all` だけがそうで、ほかは一致しない接続先がありうる。
func blockAppliesEverywhere(block Block) bool {
	switch block.Kind {
	case BlockHost:
		everyHost := false
		for _, pattern := range block.Patterns {
			if pattern.Negated {
				return false
			}
			everyHost = everyHost || pattern.Value == "*"
		}
		return everyHost
	case BlockMatch:
		return len(block.Criteria) == 1 && block.Criteria[0].Keyword == "all" && !block.Criteria[0].Negated
	default:
		return true
	}
}
