//go:build windows

package nativepath

import (
	"strings"
	"unicode"
)

// foldIdentity は、Windows の大小文字同一視に合わせて鍵をひとつに畳む。
//
// **strings.ToLower ではない。** 包含の判断は filepath.Rel を通り、その中身は
// strings.EqualFold である。両者は一致しない組を持ち、しかも**両方向へ**ずれる。
// 片方向——EqualFold は同じと言うのに ToLower が分ける組——は、同じファイルが
// 二つの節点として現れるだけで済む。もう片方は済まない。EqualFold が別だと言う
// 二つのファイルを ToLower が同じ鍵に畳むと、二つ目は重複として扱われ、
// **一度も読まれないまま黙って消える。**
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
