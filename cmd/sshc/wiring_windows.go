//go:build windows

package main

import (
	"os"

	"sshc/internal/keys"
	"sshc/internal/platform/windows"
)

// newPlatformParts は、この OS の部品を組み立てる。
//
// Toolchain は Windows 自身が置いた OpenSSH だけを指す。**PATH は渡さない** ——
// Windows の PATH には利用者が書き込めるディレクトリが並び、その一本が鍵の生成を
// 引き受ければ、生成された鍵はもう利用者のものではない。信頼の起点は %SystemRoot%
// であり、それを読むのはこの配線の仕事である。internal/platform/windows は環境変数
// を知らないままでいる。
//
// KeyAgent は Windows の OpenSSH エージェントが待つ固定の named pipe へ話す。
// lookup を渡すのは Unix と同じ signature を保つためだけで、**あちらはそれを
// 読まない。**
func newPlatformParts() platformParts {
	return platformParts{
		Toolchain: windows.NewToolchain(systemRoot()),
		KeyAgent:  keys.NewAgent(os.LookupEnv),
	}
}

// systemRoot は Windows ディレクトリの綴りを返す。
//
// 正しい綴りは SystemRoot である。windir も同じ場所を指すが、こちらは利用者の
// 環境に残っていることがある古い綴りなので、後ろに置く。どちらも無いなら空を
// 返す —— NewToolchain は空を「起点が無い」として扱い、相対パスを組み立てない。
func systemRoot() string {
	if root := os.Getenv("SystemRoot"); root != "" {
		return root
	}
	return os.Getenv("windir")
}
