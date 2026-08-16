//go:build unix

package acceptance_test

import "os"

// builtBinaryName は、このリポジトリが作る実行ファイルの名前である。
const builtBinaryName = "sshc"

// isolatedEnvironment は、子プロセスに渡す環境をまるごと置き換える。
//
// **本物のホームを読ませないための境界である。** 継承した環境をそのまま渡すと、
// 何を読んだかがそのマシンの設定次第になる。HOME と PATH の二つだけを与える。
func isolatedEnvironment(home string) []string {
	return []string{"HOME=" + home, "PATH=" + os.Getenv("PATH")}
}
