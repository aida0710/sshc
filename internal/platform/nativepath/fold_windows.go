//go:build windows

package nativepath

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// foldIdentity は、Windows の大小文字同一視に合わせて鍵をひとつに畳む。
//
// strings.ToLower ではない。包含の判断は filepath.Rel を通り、その中身は
// strings.EqualFold である。両者は一致しない組を持ち、しかも両方向へずれる。
// EqualFold は同じと判定しても ToLower が区別する組み合わせでは、同じファイルが
// 二つの節点として現れるだけで済む。もう片方は済まない。EqualFold が別だと言う
// 二つのファイルを ToLower が同じ鍵に畳むと、二つ目は重複として扱われ、
// 一度も読まれないまま暗黙に消える。
//
// SimpleFold の軌道の最小値を取れば、EqualFold が等しいと言う組はちょうど同じ
// 鍵になる。EqualFold の定義がその軌道そのものだからである。
func foldIdentity(path string) string {
	var builder strings.Builder
	builder.Grow(len(path))
	for _, letter := range path {
		builder.WriteRune(foldRune(letter))
	}
	return builder.String()
}

// foldRune は、その文字が属する simple fold の軌道の最小値を返す。
func foldRune(letter rune) rune {
	minimum := letter
	for folded := unicode.SimpleFold(letter); folded != letter; folded = unicode.SimpleFold(folded) {
		if folded < minimum {
			minimum = folded
		}
	}
	return minimum
}

// matchPrefix は、区切り文字の違いと、foldIdentity と同じ大小文字の同一視を
// 許して比べる。区切り文字を同一視するのは、Identity が通す filepath.Clean が
// `/` を `\` に揃えるからである。
func matchPrefix(text, path string) (int, bool) {
	consumed := 0
	for _, want := range path {
		got, size := utf8.DecodeRuneInString(text[consumed:])
		if size == 0 || !sameLetter(got, want) {
			return 0, false
		}
		consumed += size
	}
	return consumed, true
}

func sameLetter(got, want rune) bool {
	if isSeparator(got) && isSeparator(want) {
		return true
	}
	return foldRune(got) == foldRune(want)
}

func isSeparator(letter rune) bool { return letter == '/' || letter == '\\' }
