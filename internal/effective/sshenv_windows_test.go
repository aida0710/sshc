//go:build windows

package effective_test

import (
	"os"
	"path/filepath"
)

// sshEnvironment は、ssh に渡す環境をまるごと置き換える。
//
// **フィクスチャのホームが子プロセスのホームでなければならない。** ssh は相対
// Include を ~/.ssh に固定し、~ をここから取るので、継承した環境のままでは
// フィクスチャの Include が本物の ~/.ssh へ到達する。Windows でそれを決めるのは
// HOME ではなく USERPROFILE なので、ホームを名乗る変数はすべて向け直す。
//
// 残りは ssh.exe が起動するために要るものだけである。SystemRoot が無ければ
// winsock の読み込みから失敗し、そこで出るのは設定とは無関係の 255 である。
func sshEnvironment(home string) []string {
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
