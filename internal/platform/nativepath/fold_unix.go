//go:build !windows

package nativepath

import "strings"

// foldIdentity は Unix では何もしない。
//
// 表記が違えば別のファイルだと言えるのは Linux の普通のファイルシステムだけである。
// macOS の既定の APFS は大小文字も Unicode の正規化（NFC と NFD）も区別しないので、
// 表記の違う二つのパスが同じファイルでありうる。それでも畳まないのは、区別する
// APFS もあり、表記だけではどちらのボリュームか分からないからである。区別する
// ボリュームで別々のファイルを同じ鍵に畳むと、二つ目の Include は重複として扱われ、
// 一度も読まれないまま暗黙に消える。間違える方向としてそれが最も悪い。
//
// 畳まない代わりに、区別しないボリュームでは次の誤りが残る。実際のディレクトリと
// 違う大小文字で Include を書くと、そのファイルはルートの外として編集できない扱いに
// なる。ルートより下だけ大小文字が違えば、同じファイルが二つの節点として二度読まれ、
// 重複の診断は出ない。自分自身を Include すると、循環はひとつ遅れて見つかる。
//
// validate.Reserved が逆に大小文字を畳むのは、名前を断る側の判断だからである。
// 断りすぎても、読まれずに消えるファイルは無い。
func foldIdentity(path string) string { return path }

// matchPrefix は Unix では表記をそのまま比べる。理由は foldIdentity と同じである。
func matchPrefix(text, path string) (int, bool) {
	if !strings.HasPrefix(text, path) {
		return 0, false
	}
	return len(path), true
}
