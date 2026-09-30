package windows

import (
	"errors"
	"path/filepath"
	"strings"
)

// ErrNoLoginShell は、信頼できる場所にシェルが一本も無いことを報告する。
var ErrNoLoginShell = errors.New("no trusted login shell was found")

// ErrNoCommandProcessor は、信頼できる場所に cmd.exe が無いことを報告する。
var ErrNoCommandProcessor = errors.New("no trusted cmd.exe was found")

// LoginShell は、埋め込みターミナルが開くシェルの絶対パスを返す。
//
// SHELL は見ない。Windows でそれを置くのは MSYS や Cygwin であり、値は
// `/usr/bin/bash` のような、Windows が起動できない表記である。利用者の選択では
// なく、何かをインストールした副作用だ。ここで信頼するのは Windows 自身が置いた
// 三つの場所だけであり、順は上から下である。
//
//  1. %ProgramFiles% の PowerShell 7
//  2. Windows に同梱された Windows PowerShell
//  3. %ComSpec%
//
// lookup が返すのは ProgramFiles、WINDIR、ComSpec の三つである。最初の
// 二つは環境から取ってはならない。そこを差し替えられれば、端末に渡るのは
// 利用者のシェルではなくなる。呼び出し側が Windows 自身に尋ねて渡す。
// stat が nil なら、実在する通常ファイルであることを確かめる。
func LoginShell(lookup func(string) (string, bool), stat func(string) error) (string, error) {
	if shell, ok := firstExisting(loginShellCandidates(lookup), stat); ok {
		return shell, nil
	}
	return "", ErrNoLoginShell
}

// LoginArguments は、そのシェルをログインシェルとして起動するための argv の残り。
//
// Unix のハイフン付き argv[0] に当たるものは Windows に無い。代わりに
// あるのは、PowerShell が起動のたびに出す著作権表示だけである。プロファイルは
// 読ませる。開いているのは利用者のシェルであって、素のインタプリタではない。
func LoginArguments(shell string) []string {
	switch strings.ToLower(programName(shell)) {
	case "pwsh.exe", "powershell.exe":
		return []string{"-NoLogo"}
	}
	return nil
}

// CommandProcessor は、cmd.exe の絶対パスを返す。
//
// cmd.exe の文法で書かれたもの（ProxyCommand の行、Command Prompt のプロファイル）
// を渡す先なので、名前が cmd.exe であることまで求める。%ComSpec% が信頼できる
// 表記で cmd.exe を指していればそれを、そうでなければ Windows ディレクトリの
// System32\cmd.exe を返す。lookup と stat の約束は LoginShell と同じである。
func CommandProcessor(lookup func(string) (string, bool), stat func(string) error) (string, error) {
	if processor, ok := firstExisting(commandProcessorCandidates(lookup), stat); ok {
		return processor, nil
	}
	return "", ErrNoCommandProcessor
}

// PowerShell7Path は、%ProgramFiles% の下で PowerShell 7 が置かれる場所を返す。
func PowerShell7Path(programFiles string) string {
	return filepath.Join(programFiles, "PowerShell", "7", "pwsh.exe")
}

// WindowsPowerShellPath は、Windows に同梱された Windows PowerShell の場所を返す。
func WindowsPowerShellPath(windowsDirectory string) string {
	return filepath.Join(windowsDirectory, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
}

func loginShellCandidates(lookup func(string) (string, bool)) []string {
	lookup = lookupOrNothing(lookup)
	var found []string
	if programFiles, ok := lookup("ProgramFiles"); ok && programFiles != "" {
		found = append(found, PowerShell7Path(programFiles))
	}
	if windowsDirectory, ok := lookup("WINDIR"); ok && windowsDirectory != "" {
		found = append(found, WindowsPowerShellPath(windowsDirectory))
	}
	if comSpec, ok := trustedComSpec(lookup); ok {
		found = append(found, comSpec)
	}
	return found
}

func commandProcessorCandidates(lookup func(string) (string, bool)) []string {
	lookup = lookupOrNothing(lookup)
	var found []string
	if comSpec, ok := trustedComSpec(lookup); ok && strings.EqualFold(programName(comSpec), "cmd.exe") {
		found = append(found, comSpec)
	}
	if windowsDirectory, ok := lookup("WINDIR"); ok && windowsDirectory != "" {
		found = append(found, filepath.Join(windowsDirectory, "System32", "cmd.exe"))
	}
	return found
}

// firstExisting は、候補のうち最初に実在するものを返す。stat が nil なら、
// 実在する通常ファイルであることを確かめる。
func firstExisting(candidates []string, stat func(string) error) (string, bool) {
	if stat == nil {
		stat = existingProgram
	}
	for _, candidate := range candidates {
		if stat(candidate) == nil {
			return candidate, true
		}
	}
	return "", false
}

func lookupOrNothing(lookup func(string) (string, bool)) func(string) (string, bool) {
	if lookup == nil {
		return func(string) (string, bool) { return "", false }
	}
	return lookup
}

// trustedComSpec は、%ComSpec% の表記を起動してよいなら、その値を返す。
//
// %ComSpec% だけが利用者の環境から来る。ProgramFiles や WINDIR と違い、これは
// 表記そのものを疑う。実在するかどうかを尋ねるのは、プログラムの形をしていると
// 分かってからである。
func trustedComSpec(lookup func(string) (string, bool)) (string, bool) {
	comSpec, ok := lookup("ComSpec")
	if !ok || !trustedProgramPath(comSpec) {
		return "", false
	}
	return comSpec, true
}

// trustedProgramPath は、環境から来た表記をそのまま起動してよいかを決める。
//
// 求めるのは、ドライブから始まる絶対パスに置かれた一本の `.exe` である。
// 相対パスは、それを解釈するプロセスの居場所で意味が変わる。引用符と引数は、
// このアプリケーションが組み立てないコマンドラインを環境に書かせることになる。
// 装置名前空間（`\\?\`、`\\.\`、`\??\`）と UNC 共有は、ローカルの Windows が
// 置いたものではない。`..` は、そこから任意の場所へ抜けられる。
func trustedProgramPath(value string) bool {
	if value == "" || strings.Contains(value, `"`) {
		return false
	}
	for _, character := range value {
		if character < 0x20 {
			return false
		}
	}
	normalized := strings.ReplaceAll(value, "/", `\`)
	if strings.HasPrefix(normalized, `\\`) || strings.HasPrefix(normalized, `\??\`) {
		return false
	}
	if len(normalized) < 4 || !isASCIILetter(normalized[0]) || normalized[1] != ':' || normalized[2] != '\\' {
		return false
	}
	for _, component := range strings.Split(normalized[3:], `\`) {
		if component == "" || component == "." || component == ".." || strings.Contains(component, ":") {
			return false
		}
	}
	return strings.HasSuffix(strings.ToLower(normalized), ".exe")
}

// programName は、Windows の表記で最後の要素を返す。filepath.Base は動いて
// いる OS の区切り文字しか知らないので、macOS の上では `\` を分けない。
func programName(path string) string {
	if index := strings.LastIndexAny(path, `\/`); index >= 0 {
		return path[index+1:]
	}
	return path
}

func isASCIILetter(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}
