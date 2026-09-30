//go:build windows

package main

import (
	"os"

	"sshc/internal/keys"
	"sshc/internal/platform"
	"sshc/internal/platform/windows"
)

// newPlatformParts は、この OS の部品を組み立てる。
//
// Toolchain は Windows 自身が置いた OpenSSH だけを指す。PATH は渡さない。
// Windows の PATH には利用者が書き込めるディレクトリが並び、その並びを決めているのは
// このアプリケーションではない。Toolchain が答えるのは、ハードウェア鍵の項目を出して
// よいかだけで、見つけたパスで鍵を生成することはない。画面に出す ssh-keygen の
// コマンドは、利用者のシェルが PATH で解決する。信頼の起点は Windows ディレクトリ
// であり、環境変数の SystemRoot ではなく Windows 自身に尋ねる
// （platform.WindowsDirectory）。尋ねられなければ空を渡し、NewToolchain はそれを
// 「起点が無い」として扱う。internal/platform/windows は環境も Win32 も知らないままでいる。
//
// KeyAgent は Windows の OpenSSH エージェントが待つ固定の named pipe へ接続する。
// lookup を渡すのは Unix と同じ signature を保つためだけで、あちらはそれを
// 読まない。
func newPlatformParts() platformParts {
	windowsDirectory, _ := platform.WindowsDirectory()
	return platformParts{
		Toolchain: windows.NewToolchain(windowsDirectory),
		KeyAgent:  keys.NewAgent(os.LookupEnv),
	}
}
