//go:build unix

// Package isolatedhome は、本物のビルドを起動するテストが、子プロセスに本物のホームを
// 読ませないための環境を作る。
package isolatedhome

import "os"

// Environment は、子プロセスに渡す環境をまるごと置き換える。
//
// 本物のホームを読ませないための境界である。継承した環境をそのまま渡すと、
// 何を読んだかがそのマシンの設定次第になる。HOME と PATH の二つだけを与える。
func Environment(home string) []string {
	return []string{"HOME=" + home, "PATH=" + os.Getenv("PATH")}
}
