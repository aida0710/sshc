//go:build windows

package platform

import (
	"golang.org/x/sys/windows"

	trusted "sshc/internal/platform/windows"
)

// LoginShell は、埋め込みターミナルが開くシェルの絶対パスを返す。
//
// 実行ビットで測れない。Windows の Perm() が運ぶのは所有者の書き込み
// ビットだけなので、Unix と同じ検査をここで走らせると、どの候補も「実行不可」に
// 見える。SHELL が設定されていても端末は一本も開かない。それが、この分割の
// 前に起きていたことである。
//
// どこから選ぶかは trusted.LoginShell が持つ。ここが足すのは、その信頼の起点を
// Windows 自身から取ることだけである。
func LoginShell(lookup func(string) (string, bool)) (string, error) {
	shell, err := trusted.LoginShell(systemLookup(lookup), nil)
	if err != nil {
		return "", ErrNoLoginShell
	}
	return shell, nil
}

// LoginArgv0 は空である。
//
// Windows に、ログインシェルを表す argv[0] は無い。先頭のハイフンは Unix の
// シェルだけが読む合図であり、PowerShell に `-powershell.exe` を渡せば、それは
// 表記を間違えた引数にしか見えない。
func LoginArgv0(string) string { return "" }

// LoginArguments は、そのシェルをログインシェルとして起動するための引数である。
func LoginArguments(shell string) []string { return trusted.LoginArguments(shell) }

// systemLookup は、シェルの在り処を Windows 自身に尋ねる。
//
// どこにシェルがあるかを環境に決めさせない。`%ProgramFiles%` を書き換え
// られる立場にあるものは、それだけで、端末に渡るプログラムを選べてしまう。
// Windows は同じことを環境変数を通さずに返す手段を持っているので、そちらを
// 使う。ここを通れる名前は三つだけで、`%ComSpec%` だけが呼び出し側の環境から
// 来る。それが利用者の選んだコマンドプロセッサだからである。
func systemLookup(lookup func(string) (string, bool)) func(string) (string, bool) {
	return func(name string) (string, bool) {
		switch name {
		case "WINDIR":
			directory, err := windows.GetSystemWindowsDirectory()
			return directory, err == nil
		case "ProgramFiles":
			directory, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, windows.KF_FLAG_DEFAULT)
			return directory, err == nil
		case "ComSpec":
			if lookup == nil {
				return "", false
			}
			return lookup(name)
		}
		return "", false
	}
}
