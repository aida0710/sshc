//go:build windows

package platform

import "golang.org/x/sys/windows"

// WindowsDirectory は、Windows ディレクトリ（通常は C:\Windows）を Windows 自身に
// 尋ねて返す。
//
// 環境変数の SystemRoot や WINDIR は読まない。そこを書き換えられる立場にあるものが、
// cmd.exe や同梱の OpenSSH として起動されるプログラムを選べてしまうからである。
// シェル、ProxyCommand、鍵の生成の信頼の起点は、どれもここから取る。
func WindowsDirectory() (string, bool) {
	directory, err := windows.GetSystemWindowsDirectory()
	return directory, err == nil && directory != ""
}
