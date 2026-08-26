package sshclient

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// ErrNoInterpreter は、ProxyCommand を解釈させる相手が居ないことを報告する。
var ErrNoInterpreter = errors.New("no command interpreter is available to run ProxyCommand")

// interpreter は、その表記を走らせるプログラムと引数を返す。
//
// `cmd.exe` である。Windows の OpenSSH もそうしており、ProxyCommand の行は
// あちらでそう解釈される前提で書かれている。PowerShell を使うと、引用と
// リダイレクトの規則が変わって同じ行が別の意味になる。
//
// PATH では探さない。探せば、`cmd.exe` という名前の別のものを起動しうる。
// Windows 自身が置いた場所だけを見る。
func interpreter(command string) (string, []string, error) {
	shell, err := commandInterpreter(os.LookupEnv)
	if err != nil {
		return "", nil, err
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

// commandInterpreter は、信頼できる cmd.exe の表記を返す。
//
// lookup を受け取るのは、この選び方を検査できるようにするためである。
func commandInterpreter(lookup func(string) (string, bool)) (string, error) {
	if lookup == nil {
		return "", ErrNoInterpreter
	}
	// %ComSpec% は Windows 自身が置く。値が絶対パスであることだけ確かめる。
	if spec, ok := lookup("ComSpec"); ok && filepath.IsAbs(spec) && existingFile(spec) {
		return spec, nil
	}
	for _, name := range []string{"SystemRoot", "WINDIR"} {
		if root, ok := lookup(name); ok && root != "" {
			candidate := filepath.Join(root, "System32", "cmd.exe")
			if existingFile(candidate) {
				return candidate, nil
			}
		}
	}
	return "", ErrNoInterpreter
}

func existingFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
