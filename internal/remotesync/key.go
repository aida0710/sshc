package remotesync

import (
	"crypto/rand"
	"encoding/base32"
	"strings"
)

// KeyBytes は、生成する鍵の素の長さ。120 ビットである。
//
// 5 の倍数であることに意味がある。base32 は 5 ビットを 1 文字にするので、
// ここが 5 の倍数のときだけ余りが出ず、詰め物の `=` が付かない。ユーザーが読んでユーザーが
// 打つ文字列に、意味を持たない記号を混ぜない。
const KeyBytes = 15

// keyGroup は、区切りを入れる間隔。
const keyGroup = 4

// keyAlphabet は Crockford の base32 である。
//
// `I` `L` `O` `U` が無い。前の三つは 1 と 0 に化けるからで、最後の一つは、
// 生成した文字列がたまたまユーザーを怒らせる語にならないためである。読み違いは、鍵を
// 別の端末へ運ぶこの用途では、そのまま「開かない」になる。
const keyAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

var keyEncoding = base32.NewEncoding(keyAlphabet).WithPadding(base32.NoPadding)

// NewKey は、リモートのスナップショットを暗号化する鍵を 1 つ作る。
//
// 既定が生成であることに理由がある。この値は端末をまたいで共有されるので、
// どこかに書き留められる。書き留めるなら、覚えられる必要はない。覚えられる必要が
// 無いなら、ユーザーが決める理由も無い。
//
// 出てくるのは `AB12-CD34-EF56-GH78-JK90-MN12` の形の 29 文字である。区切りも鍵の
// 一部である。表示された通りに入力すれば通ることを唯一の規則とするため、
// 打った文字を後から削る処理を入れない。
func NewKey() (string, error) {
	raw := make([]byte, KeyBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	encoded := keyEncoding.EncodeToString(raw)
	var out strings.Builder
	for index := 0; index < len(encoded); index += keyGroup {
		if index > 0 {
			out.WriteByte('-')
		}
		out.WriteString(encoded[index : index+keyGroup])
	}
	return out.String(), nil
}
