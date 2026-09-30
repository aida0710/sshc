package sshclient

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"sshc/internal/platform"
)

// ErrNoInterpreter は、ProxyCommand を解釈させる相手が居ないことを報告する。
var ErrNoInterpreter = errors.New("no command interpreter is available to run ProxyCommand")

// interpreter は、その表記を走らせるプログラムと引数を返す。
//
// `cmd.exe` である。Windows の OpenSSH もそうしており、ProxyCommand の行は
// あちらでそう解釈される前提で書かれている。PowerShell を使うと、引用と
// リダイレクトの規則が変わって同じ行が別の意味になる。
//
// どの cmd.exe を信頼するかは platform.CommandProcessor が決める。PATH では
// 探さず、%ComSpec% は表記を確かめたうえで cmd.exe を指すときだけ使い、それ以外は
// Windows 自身に尋ねた Windows ディレクトリの cmd.exe を使う。ローカルシェルの
// Command Prompt と同じ規則である。
func interpreter(command string) (string, []string, error) {
	shell, err := platform.CommandProcessor(os.LookupEnv)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %w", ErrNoInterpreter, err)
	}
	// `exec` に当たるものは cmd.exe に無い。/c は「これを走らせて終わる」
	// なので、シェルが待つためだけに残ることはない。
	return shell, []string{"/d", "/s", "/c", command}, nil
}

// cmd.exeはCommandLineToArgvWと異なるquote規則を使う。os/execにargvの
// escapeを任せるとcommand内のdouble quoteがbackslash付きで渡り、空白を含む
// executable pathを起動できない。shellへ渡す部分だけをraw command lineにする。
func configureProxyCommandProcess(command *exec.Cmd, line string) {
	command.Args = nil
	command.SysProcAttr = &syscall.SysProcAttr{CmdLine: `/d /s /c "` + line + `"`}
}
