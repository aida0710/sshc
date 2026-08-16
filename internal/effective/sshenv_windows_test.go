//go:build windows

package effective_test

import (
	"os"
	"path/filepath"
)

// sshEnvironment は、ssh に読ませるホームだけを差し替える。
//
// **フィクスチャのホームが子プロセスのホームでなければならない。** ssh は相対
// Include を ~/.ssh に固定し、~ をここから取るので、そのままではフィクスチャの
// Include が本物の ~/.ssh へ到達する。Windows でそれを決めるのは HOME ではなく
// USERPROFILE なので、ホームを名乗る変数はすべて向け直す。
//
// Unix 側と違い、環境をまるごと置き換えることはしない。ssh.exe は Go の
// プログラムではなく、SystemRoot や ProgramData を含む一式が揃わないと、設定と
// 何の関係もない 255 で終わる——そして何も言わない。読む設定を決めるのは `-F` と
// ホームだけなので、残りを継承しても、どの設定を読むかは変わらない。名前の
// 重複は問題にならない。Windows の環境ブロックは大小文字を区別せず、Go は
// 後から来たものを採る。
func sshEnvironment(home string) []string {
	return append(os.Environ(),
		"HOME="+home,
		"USERPROFILE="+home,
		"HOMEDRIVE="+filepath.VolumeName(home),
		"HOMEPATH="+home[len(filepath.VolumeName(home)):],
	)
}
