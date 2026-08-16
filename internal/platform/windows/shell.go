package windows

import (
	"errors"
	"path/filepath"
	"strings"
)

// ErrNoLoginShell は、信頼できる場所にシェルが一本も無いことを報告する。
var ErrNoLoginShell = errors.New("no trusted login shell was found")

// LoginShell は、埋め込みターミナルが開くシェルの絶対パスを返す。
//
// **SHELL は見ない。** Windows でそれを置くのは MSYS や Cygwin であり、値は
// `/usr/bin/bash` のような、Windows が起こせない綴りである。利用者の選択では
// なく、何かをインストールした副作用だ。ここで信頼するのは Windows 自身が置いた
// 三つの場所だけであり、順は上から下である。
//
//  1. %ProgramFiles% の PowerShell 7
//  2. Windows に同梱された Windows PowerShell
//  3. %ComSpec%
//
// lookup が答えるのは ProgramFiles、WINDIR、ComSpec の三つである。**最初の
// 二つは環境から取ってはならない**——そこを差し替えられれば、端末に渡るのは
// 利用者のシェルではなくなる。呼び出し側が Windows 自身に尋ねて渡す。
// stat が nil なら、実在する通常ファイルであることを確かめる。
func LoginShell(lookup func(string) (string, bool), stat func(string) error) (string, error) {
	if stat == nil {
		stat = existingProgram
	}
	for _, candidate := range candidates(lookup) {
		if stat(candidate) == nil {
			return candidate, nil
		}
	}
	return "", ErrNoLoginShell
}

// LoginArguments は、そのシェルをログインシェルとして起こすための argv の残り。
//
// **Unix のハイフン付き argv[0] に当たるものは Windows に無い。** 代わりに
// あるのは、PowerShell が起動のたびに出す著作権表示だけである。プロファイルは
// 読ませる——開いているのは利用者のシェルであって、素のインタプリタではない。
func LoginArguments(shell string) []string {
	switch strings.ToLower(programName(shell)) {
	case "pwsh.exe", "powershell.exe":
		return []string{"-NoLogo"}
	}
	return nil
}

func candidates(lookup func(string) (string, bool)) []string {
	if lookup == nil {
		lookup = func(string) (string, bool) { return "", false }
	}
	var found []string
	if programFiles, ok := lookup("ProgramFiles"); ok && programFiles != "" {
		found = append(found, filepath.Join(programFiles, "PowerShell", "7", "pwsh.exe"))
	}
	if windowsDirectory, ok := lookup("WINDIR"); ok && windowsDirectory != "" {
		found = append(found, filepath.Join(
			windowsDirectory, "System32", "WindowsPowerShell", "v1.0", "powershell.exe",
		))
	}
	// **%ComSpec% だけが利用者の環境から来る。** 上の二つと違い、これは綴りその
	// ものを疑う。実在するかどうかを尋ねるのは、プログラムの形をしていると
	// 分かってからである。
	if comSpec, ok := lookup("ComSpec"); ok && trustedProgramPath(comSpec) {
		found = append(found, comSpec)
	}
	return found
}

// trustedProgramPath は、環境から来た綴りをそのまま起こしてよいかを決める。
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

// programName は、Windows の綴りで最後の要素を返す。filepath.Base は動いて
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
