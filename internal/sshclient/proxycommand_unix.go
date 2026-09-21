//go:build !windows

package sshclient

import (
	"errors"
	"os/exec"
)

// posixShell は、ProxyCommand を解釈させるシェルである。
//
// コマンドの引用やリダイレクトはPOSIXの規則で解釈する。engineがログイン
// シェルからPATHを取得した場合も、そのシェルの文法へ切り替えない。
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

func configureProxyCommandProcess(_ *exec.Cmd, _ string) {}
