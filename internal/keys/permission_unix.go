//go:build unix

package keys

import "io/fs"

// exposedToOthers は、その鍵を持ち主以外が読めるかを言う。
//
// Unix では mode ビットがそのまま答えである。
func exposedToOthers(info fs.FileInfo) bool {
	return info.Mode().Perm()&0o077 != 0
}
