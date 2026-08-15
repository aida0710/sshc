//go:build windows

package nativepath

import "strings"

// foldIdentity は、Windows の大小文字同一視に合わせて鍵をひとつに畳む。
//
// filepath.Rel が包含判断に使う strings.EqualFold と、ASCII のパスでは常に
// 一致する。両者が分かれるのは、単純な case fold で等しくなるのに ToLower では
// 分かれる非 ASCII の組だけであり、そのときは同じファイルが二つの節点として
// 現れる — 重複の報告が増えるだけで、外のパスに手が届くことはない。
func foldIdentity(path string) string { return strings.ToLower(path) }
