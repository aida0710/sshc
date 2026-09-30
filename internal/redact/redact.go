// Package redact は、利用者が渡した既知のシークレットの値を、画面・ログ・transcript へ
// 出す文字列から伏せる。
//
// シークレットが複数あると、一方が他方の一部（頭が同じなど）になっていることがある。
// 値を順に置き換えるやり方では、短い方が先に一致した時点で長い方の残りがそのまま出る。
// ここでは、すべての値の一致範囲を元の文字列の上で集めてから、重なる範囲と隣り合う範囲を
// まとめ、まとまりごとに印を 1 つ置く。どの順で渡しても結果は変わらない。
package redact

import (
	"bytes"
	"sort"
)

// Secret は、伏せる値と、それを探す範囲である。
type Secret struct {
	Value []byte
	// From より前では Value を探さない。値を送る前に出ていた同じ文字列は、
	// 利用者自身が見ていた出力であり、シークレットの漏れではないからである。
	From int
	// CutAt が 0 より大きいとき、その位置で終わる Value の頭（Value より短いもの）も伏せる。
	// 出力を上限で切った位置や、値を送った直後の行末のように、値が途中で切れうる位置を渡す。
	// 頭の直前（From より後）が英数字か "_" のときは、単語の途中とみなして伏せない。そうしないと、
	// シークレットが "s" で始まるだけで "shell ready" のような普通の出力が崩れる。
	CutAt int
}

// Bytes は、text の中の secrets の値を mark に置き換えた写しを返す。text は変更しない。
// バイト列のまま扱うので、不正な UTF-8 を混ぜても一致を避けられない。
func Bytes(text []byte, secrets []Secret, mark string) []byte {
	spans := matchedSpans(text, secrets)
	if len(spans) == 0 {
		return append([]byte(nil), text...)
	}
	redacted := make([]byte, 0, len(text))
	previousEnd := 0
	for _, span := range mergeSpans(spans) {
		redacted = append(redacted, text[previousEnd:span.start]...)
		redacted = append(redacted, mark...)
		previousEnd = span.end
	}
	return append(redacted, text[previousEnd:]...)
}

// Values は、text の中の values を mark に置き換える。探す範囲も切れた頭も考えない、
// いちばん単純な使い方のためのもの。
func Values(text string, values []string, mark string) string {
	secrets := make([]Secret, 0, len(values))
	for _, value := range values {
		secrets = append(secrets, Secret{Value: []byte(value)})
	}
	return string(Bytes([]byte(text), secrets, mark))
}

// span は、text の中で伏せる半開区間 [start, end) である。
type span struct{ start, end int }

func matchedSpans(text []byte, secrets []Secret) []span {
	var spans []span
	for _, secret := range secrets {
		if len(secret.Value) == 0 {
			continue
		}
		from := min(max(secret.From, 0), len(text))
		// 同じ値どうしが重なって現れる場合（"aaa" の中の "aa"）も両方を拾うため、
		// 一致した位置の 1 つ先から探し直す。
		for offset := from; offset < len(text); {
			found := bytes.Index(text[offset:], secret.Value)
			if found < 0 {
				break
			}
			start := offset + found
			spans = append(spans, span{start: start, end: start + len(secret.Value)})
			offset = start + 1
		}
		if cut, ok := cutPrefixSpan(text, secret, from); ok {
			spans = append(spans, cut)
		}
	}
	return spans
}

// cutPrefixSpan は、CutAt で終わる Value の頭のうち最も長いものの範囲を返す。
func cutPrefixSpan(text []byte, secret Secret, from int) (span, bool) {
	cutAt := min(secret.CutAt, len(text))
	for length := min(len(secret.Value)-1, cutAt-from); length > 0; length-- {
		start := cutAt - length
		if start > from && isWordByte(text[start-1]) {
			continue
		}
		if bytes.Equal(text[start:cutAt], secret.Value[:length]) {
			return span{start: start, end: cutAt}, true
		}
	}
	return span{}, false
}

// mergeSpans は、重なる区間と隣り合う区間を 1 つにまとめ、始まりの順に返す。
// 隣り合う区間もまとめるのは、ある値のすぐ後に別の値が続いたときに、どこで区切れるかを残さないためである。
func mergeSpans(spans []span) []span {
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	merged := []span{spans[0]}
	for _, next := range spans[1:] {
		last := &merged[len(merged)-1]
		if next.start <= last.end {
			last.end = max(last.end, next.end)
			continue
		}
		merged = append(merged, next)
	}
	return merged
}

func isWordByte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' || value == '_'
}
