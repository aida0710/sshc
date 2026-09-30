package vpnrefusal

import (
	"runtime"
	"strings"
)

// 断った理由の文に添える CLI のコマンドを、ターミナルへそのまま貼り付けられる形にする。
//
// 文を作る sshcエンジンと、貼り付けて動かす CLI は同じマシンで動くので、このマシンの
// シェルに合わせて引用する。

// ProfileCommand は、プロファイル name に action（logs、edit など）を行う CLI の
// コマンドを返す。空白や記号を含む名前は引用符で囲む。
func ProfileCommand(action, name string) string {
	return "sshc vpn " + action + " " + shellWord(runtime.GOOS, name)
}

// shellWord は、name を goos のシェルが1語と読む形にする。英数字と `.`、`-`、`_` だけの
// 名前は、どのシェルでもそのまま1語になるので、そのまま返す。
func shellWord(goos, name string) string {
	if isPlainWord(name) {
		return name
	}
	if goos == "windows" {
		return windowsShellWord(name)
	}
	return posixShellWord(name)
}

func isPlainWord(name string) bool {
	if name == "" {
		return false
	}
	for _, character := range name {
		if !isPlainCharacter(character) {
			return false
		}
	}
	return true
}

func isPlainCharacter(character rune) bool {
	return character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
		character >= '0' && character <= '9' || character == '.' || character == '-' || character == '_'
}

// posixShellWord は、name を POSIX のシェルが1語と読む形にする。一重引用符の中では
// どの字も特別な意味を持たない。一重引用符そのものは、引用を閉じて `"'"` を挟み、
// 引用を開き直して書く。
func posixShellWord(name string) string {
	return "'" + strings.ReplaceAll(name, "'", `'"'"'`) + "'"
}

// windowsExpandedInDoubleQuotes は、二重引用符の中でも特別な意味を持つ字である。
// PowerShell は `$` とバッククォートを展開し、cmd.exe は `%` を展開する。`"` は引用を閉じる。
const windowsExpandedInDoubleQuotes = "\"$`%"

// windowsShellWord は、name を PowerShell と cmd.exe が1語と読む形にする。
//
// 二重引用符は、PowerShell と cmd.exe のどちらでも空白を含む語を1語にする。pages の
// 案内（`sshc vpn up "研究室 VPN"`）もこの形である。二重引用符の中でも展開される字を
// 含む名前は、PowerShell の一重引用符（中の字を展開しない。一重引用符は2つ重ねて書く）
// で囲む。cmd.exe は一重引用符を引用として扱わないので、この場合は Windows の既定の
// シェルである PowerShell へ貼り付ける前提になる。
func windowsShellWord(name string) string {
	if !strings.ContainsAny(name, windowsExpandedInDoubleQuotes) {
		return `"` + name + `"`
	}
	return "'" + strings.ReplaceAll(name, "'", "''") + "'"
}
