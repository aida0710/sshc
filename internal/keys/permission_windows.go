//go:build windows

package keys

import (
	"io/fs"

	"sshc/internal/platform/windowsacl"
)

// exposedToOthers は、その鍵を持ち主以外が読めるかを言う。
//
// **mode ビットは見ない。** Go は Windows の通常ファイルに 0666、読み取り専用に
// 0444 を合成して返すだけで、そこに誰が読めるかは入っていない。Unix と同じ式で
// 見れば秘密鍵は必ず「危険」になり、**常に真の警告は、警告を無視することを
// 教える。**
//
// 誰が読めるかを決めているのは DACL である。所有者・SYSTEM・Administrators 以外に
// 読み取りが許可されていれば、それは露出している。
//
// **確かめられなかったときは、危険の側へ倒す。** 読めない ACL や開けない
// ファイルについて「安全です」と言うのは、この画面が最もしてはならないことで
// ある。それを見た人は、露出した鍵をそのまま使い続ける。
func exposedToOthers(path string, _ fs.FileInfo) bool {
	exposed, err := windowsacl.ReadableByOthers(path)
	if err != nil {
		return true
	}
	return exposed
}
