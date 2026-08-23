//go:build !windows

package sshclient

import "errors"

// posixShell は、ProxyCommand を解釈させるシェルである。
//
// `$SHELL` を読まない。OpenSSH はそれを見てから /bin/sh へ落ちるが、
// あちらは利用者の端末から起きたプロセスである。この engine は tmux の中にも
// systemd の下にも居る 。そこの `$SHELL` は、その supervisor がたまたま
// 持っていた値であって、利用者が選んだものではない。読めば、同じ設定が
// 「engine をどう起動したか」で違う振る舞いをする。
//
// /bin/sh は OpenSSH 自身の落とし先でもあり、ProxyCommand の行はどれも
// POSIX のシェルで書かれている。
const posixShell = "/bin/sh"

// ErrNoInterpreter は、ProxyCommand を解釈させる相手が居ないことを報告する。
var ErrNoInterpreter = errors.New("no shell is available to run ProxyCommand")

// interpreter は、その表記を走らせるプログラムと引数を返す。
//
// `exec` を前に置く。これが無いと、シェルは子を待つためだけに残る
// 接続ひとつにつきプロセスが 1 つ余分に実行を続ける。OpenSSH も同じことをする。
func interpreter(command string) (string, []string, error) {
	return posixShell, []string{"-c", "exec " + command}, nil
}
