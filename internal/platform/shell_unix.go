//go:build !windows

package platform

import (
	"os"
	"path/filepath"
	"runtime"
)

// shellFallbacks は、SHELL が無いときに順に確かめる場所である。
//
// /etc/passwd は読まない。ログイン項目として起動した常駐プロセスの中では
// SHELL が設定されていないことがあり、そこが唯一の権威になってしまうが、
// コンテナや合成 passwd の中でその一行は当てにならない。ここに並んでいるのは
// 実際に存在を確かめる絶対パスだけである。
//
// **Android には /bin が無い。** sh は /system/bin/sh (mksh) にあり、bash も
// zsh も居ない。ここに /bin/sh を残しても永久に見つからない。
//
// goos を引数で受けるのは、この一覧がテストできるようにするためである。
// runtime.GOOS をここで読むと、走っているマシンでしか通らない表明になる。
func shellFallbacks(goos string) []string {
	switch goos {
	case "darwin":
		return []string{"/bin/zsh", "/bin/bash", "/bin/sh"}
	case "android":
		return []string{"/system/bin/sh"}
	case "ios":
		// **iOS はプロセスを起こせない。** サンドボックスが fork も exec も
		// 禁じているので、置いてあるかどうか以前に、どのシェルも開けない。
		//
		// **空を返すことに意味がある。** 候補を並べれば、埋め込みターミナルは
		// 存在しない道を毎回探し、開けなかった理由を「見つからない」と言う
		// ——本当の理由は「この OS では開けない」であり、それは探して分かる
		// ことではない。SSH の接続は純粋な Go の通信なので、そちらは動く。
		return nil
	default:
		return []string{"/bin/bash", "/bin/zsh", "/bin/sh"}
	}
}

// LoginShell は、埋め込みターミナルが開くシェルの絶対パスを返す。
//
// SHELL を先に見るのは、それが利用者自身の選択だからである。ただし相対パスと、
// 存在しないパスは受け取らない。PATH 経由で解決されるシェルを起こせば、
// このアプリケーションが選んだのではないプログラムに端末を渡すことになる。
func LoginShell(lookup func(string) (string, bool)) (string, error) {
	if lookup != nil {
		if value, ok := lookup("SHELL"); ok && filepath.IsAbs(value) && executable(value) {
			return value, nil
		}
	}
	for _, candidate := range shellFallbacks(runtime.GOOS) {
		if executable(candidate) {
			return candidate, nil
		}
	}
	return "", ErrNoLoginShell
}

// LoginArgv0 は、ログインシェルとして起こすための argv[0] である。
//
// 先頭のハイフンが、そのシェルにログインシェルとしての起動を伝える。これが
// 無いと、macOS の Terminal が開くシェルとは別のファイル群が読まれ、利用者の
// PATH やプロンプトが手元の端末と食い違う。
func LoginArgv0(shell string) string { return "-" + filepath.Base(shell) }

// LoginArguments は空である。ログインシェルであることは argv[0] が伝えるので、
// 渡すべき引数が無い。
func LoginArguments(string) []string { return nil }

func executable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode().Perm()&0o111 != 0
}
