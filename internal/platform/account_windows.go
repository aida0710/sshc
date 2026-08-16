//go:build windows

package platform

import "strings"

// LocalAccountName は、OpenSSH の `%u` が指す名前を返す。
//
// **Windows の user.Current は `HOST\name` を返す。** OpenSSH がそこで `%u` に
// 入れるのは name だけである。修飾のまま渡すと、`Include conf.d/%u.conf` は
// 区切り文字をひとつ余計に含んだ別の場所を指し、`Match user` は決して一致しない。
// `\` はアカウント名に使えない文字なので、最後のそれより後ろが名前である。
func LocalAccountName(name string) string {
	return name[strings.LastIndexByte(name, '\\')+1:]
}
