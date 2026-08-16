//go:build windows

package keys

import "io/fs"

// exposedToOthers は、Windows では**答えない**。
//
// mode ビットには誰が読めるかが入っていない。Go は通常ファイルに 0666、
// 読み取り専用に 0444 を合成して返すだけで、どちらも `&0o077` に掛かる。
// つまりこれを Unix と同じ式で見ると、**すべての秘密鍵が必ず「危険」になる。**
// 常に真の警告は何も伝えず、利用者に警告そのものを無視することを教える。
//
// **知らないことを、知っているふりで言わない。** ここで本当に答えるべき問いは
// 「所有者・SYSTEM・Administrators 以外に読み取りが許可されているか」であり、
// それは DACL を歩いて deny と継承まで見る別の検査である。それを持たないうちは、
// 判断しなかったことを false で表す。
func exposedToOthers(fs.FileInfo) bool { return false }
