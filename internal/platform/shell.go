package platform

import (
	"errors"
	"strings"
)

// ErrNoLoginShell は、起動できるシェルがこのマシンに見つからないことを報告する。
var ErrNoLoginShell = errors.New("no login shell was found")

// シェルをどこから選び、どう起動するかは OS ごとに違う。同じ規則で書けるふりを
// しない。Unix のログインシェルは SHELL と `/bin` の下の絶対パスにあり、
// ハイフン付きの argv[0] で起きる。Windows にはそのどれも無く、実行してよいか
// どうかを実行ビットで測ることもできない。それぞれの規則は shell_unix.go と
// shell_windows.go にある。ここにあるのは、両方で同じであるものだけだ。

// LoginEnvironment は、ログインシェルへ渡してよい環境だけを残す。
//
// 端末は、それを起動したものの事情を継がない。開始ディレクトリと同じ話で
// ある。常駐プロセスの環境は、それを起動したものがたまたま持っていたもので
// あり、利用者はそのどれも選んでいない。npm run から起動されていれば、npm は
// 自分の設定を環境に詰めて渡してくる。`npm_config_prefix` は npm に渡した
// `--prefix` の写しであり、それを継いだシェルの中で nvm は「知らない prefix
// だ」と警告する。ここで開くのは利用者のシェルであって、ビルドの子ではない。
//
// 消すだけで足りる。起動するのはログインシェルなので、ユーザー本人が本当に設定して
// いるものは、そのシェルが読む rc がもう一度設定する。
//
// 落とすのは小文字の `npm_` だけである。npm が輸出するのはそれであり、
// NPM_TOKEN のような大文字はユーザーが自分で置いたものだからだ。
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
