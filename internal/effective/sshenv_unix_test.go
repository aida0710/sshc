//go:build unix

package effective_test

import "os"

// sshEnvironment は、ssh に渡す環境をまるごと置き換える。
//
// **フィクスチャのホームが子プロセスのホームでなければならない。** ssh は相対
// Include を ~/.ssh に固定し、~ をここから取るので、継承した環境のままでは
// フィクスチャの Include が本物の ~/.ssh へ到達する。
func sshEnvironment(home string) []string {
	return []string{"HOME=" + home, "PATH=" + os.Getenv("PATH")}
}
