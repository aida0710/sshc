package platform

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ErrNoLoginShell は、起こせるシェルがこのマシンに見つからないことを報告する。
var ErrNoLoginShell = errors.New("no login shell was found")

// shellFallbacks は、SHELL が無いときに順に確かめる場所である。
//
// /etc/passwd は読まない。ログイン項目として起動した常駐プロセスの中では
// SHELL が設定されていないことがあり、そこが唯一の権威になってしまうが、
// コンテナや合成 passwd の中でその一行は当てにならない。ここに並んでいるのは
// 実際に存在を確かめる絶対パスだけである。
func shellFallbacks() []string {
	if runtime.GOOS == "darwin" {
		return []string{"/bin/zsh", "/bin/bash", "/bin/sh"}
	}
	return []string{"/bin/bash", "/bin/zsh", "/bin/sh"}
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
	for _, candidate := range shellFallbacks() {
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

// LoginEnvironment は、ログインシェルへ渡してよい環境だけを残す。
//
// **端末は、それを起こしたものの事情を継がない。** 開始ディレクトリと同じ話で
// ある——常駐プロセスの環境は、それを起こしたものがたまたま持っていたもので
// あり、利用者はそのどれも選んでいない。npm run から起こされていれば、npm は
// 自分の設定を環境に詰めて渡してくる。`npm_config_prefix` は npm に渡した
// `--prefix` の写しであり、それを継いだシェルの中で nvm は「知らない prefix
// だ」と警告する。ここで開くのは利用者のシェルであって、ビルドの子ではない。
//
// **消すだけで足りる。** 起こすのはログインシェルなので、本人が本当に設定して
// いるものは、そのシェルが読む rc がもう一度設定する。
//
// 落とすのは小文字の `npm_` だけである。npm が輸出するのはそれであり、
// NPM_TOKEN のような大文字は人が自分で置いたものだからだ。
func LoginEnvironment(environ []string) []string {
	kept := make([]string, 0, len(environ))
	for _, entry := range environ {
		name, _, found := strings.Cut(entry, "=")
		if found && (strings.HasPrefix(name, "npm_") || name == "INIT_CWD" || name == "NODE") {
			continue
		}
		kept = append(kept, entry)
	}
	return kept
}

func executable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode().Perm()&0o111 != 0
}
