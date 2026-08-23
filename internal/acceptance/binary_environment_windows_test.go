//go:build windows

package acceptance_test

import (
	"os"
	"path/filepath"
)

// builtBinaryName は、このリポジトリが作る実行ファイルの名前である。
const builtBinaryName = "sshc.exe"

// isolatedEnvironment は、子プロセスに渡す環境をまるごと置き換える。
//
// Windows のホームは HOME ではない。os.UserHomeDir が読むのは USERPROFILE
// であり、HOME だけを差し替えても本物のプロファイルを読みに行く。ホームを名乗る
// 変数はすべて同じ一時ディレクトリへ向け、一時領域も同じ場所に閉じ込める。
//
// 残りは Windows がプロセスを起動するために要るものだけに絞る。SystemRoot が
// 無ければ winsock の読み込みから失敗し、PATHEXT が無ければ拡張子の解決が
// 変わる。名前は大小文字を問わず一意にする。Windows の環境ブロックは大小文字を
// 区別せず、同じ名前が二つあればどちらが効くかは決まらない。
func isolatedEnvironment(home string) []string {
	systemRoot := os.Getenv("SystemRoot")
	return []string{
		"HOME=" + home,
		"USERPROFILE=" + home,
		"HOMEDRIVE=" + filepath.VolumeName(home),
		"HOMEPATH=" + home[len(filepath.VolumeName(home)):],
		"LOCALAPPDATA=" + filepath.Join(home, "AppData", "Local"),
		"APPDATA=" + filepath.Join(home, "AppData", "Roaming"),
		"TEMP=" + filepath.Join(home, "Temp"),
		"TMP=" + filepath.Join(home, "Temp"),
		"SystemRoot=" + systemRoot,
		"windir=" + systemRoot,
		"ComSpec=" + os.Getenv("ComSpec"),
		"PATHEXT=" + os.Getenv("PATHEXT"),
		"PATH=" + os.Getenv("PATH"),
	}
}
