//go:build unix

package keys

import "io/fs"

// exposedToOthers は、その鍵を持ち主以外が読めるかを言う。
//
// Unix では mode ビットがそのまま結果である。道は要らない。
// 結果は既に手元の info の中にある。Windows は逆で、そちらは道から DACL を
// 引かなければ何も言えない。
func exposedToOthers(_ string, info fs.FileInfo) bool {
	return info.Mode().Perm()&0o077 != 0
}
