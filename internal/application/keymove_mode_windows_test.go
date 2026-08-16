//go:build windows

package application

import "testing"

// Windows に POSIX の permission bit は無い。os.Chmod が写すのは所有者の
// 書き込みビットひとつだけで、Mode().Perm() が返すのは常に 0666 か 0444 で
// ある。0400 を書いても読み返しても、確かめているのは値の丸めであって、
// 移動が rename であったかどうかではない。
//
// ここで秘密を守っているのは DACL であり、その契約は
// internal/platform/windowsacl が持っている。ただし鍵の移動先 ~/.ssh/keys は
// private state ではないので、トランザクションはそこに DACL を適用しない
// ——**この移動には、確かめるべき Windows 側の対応物が存在しない。**
// 同じ検査の残り（バックアップに鍵材料が届かないこと）はどちらの OS でも走る。
func markKeyMode(*testing.T, string) {}

func assertKeyModeSurvived(*testing.T, string) {}
