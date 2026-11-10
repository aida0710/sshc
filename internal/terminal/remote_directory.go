package terminal

import (
	"errors"
	"path"
	"strings"
)

var ErrInvalidRemoteWorkingDirectory = errors.New("remote working directory is invalid")

// CheckRemoteWorkingDirectory は、SSH のサーバーの POSIX の絶対パスのフォルダか、シェルがタイトルで報告する
// 形の、リモートのホームの下のフォルダ（`~`、`~/src`）を受け入れる。
func CheckRemoteWorkingDirectory(candidate string) (string, error) {
	if !isDirectoryText(candidate) {
		return "", ErrInvalidRemoteWorkingDirectory
	}
	underHome, isUnderHome := strings.CutPrefix(candidate, "~/")
	switch {
	case candidate == "~":
		return candidate, nil
	case isUnderHome:
		if path.Clean(underHome) != underHome || strings.HasPrefix(underHome, "/") || strings.HasPrefix(underHome, "../") || underHome == ".." {
			return "", ErrInvalidRemoteWorkingDirectory
		}
		return candidate, nil
	case strings.HasPrefix(candidate, "/") && path.Clean(candidate) == candidate:
		return candidate, nil
	}
	return "", ErrInvalidRemoteWorkingDirectory
}

// shellSpecialCharacters は、SSH のサーバーが動かしうるログインシェルのどれかで、単一引用符の中でもただの
// 文字にならない文字である。sh、bash、zsh は `'` で引用を終える。fish は引用の中の `\` も読む。PowerShell は
// U+2018 から U+201B の引用符でも引用を終える。cmd.exe には単一引用符が無いので、演算子と展開
// （`&|<>^%!"`）がどこでも働く。バッククォートは PowerShell のエスケープ文字である。
const shellSpecialCharacters = "'\\\"`&|<>^%!‘’‚‛"

// ChangeRemoteDirectoryCommand は、CheckRemoteWorkingDirectory が受け入れたフォルダへ、リモートのシェルを
// 移すコマンドである。シェルがホームフォルダに展開するよう、`~` は引用符の外に置く。
//
// サーバーのログインシェルは分からないので、フォルダがどのシェルでも同じ意味になるときだけ、コマンドを
// 打ち込む。そうでなければ false を返し、セッションはホームフォルダで始まる。フォルダを決めるのはシェルの
// 出力で、あるシェルの流儀で引用すると、別のシェルでは引用が終わり、残りがコマンドとして走りうる。
func ChangeRemoteDirectoryCommand(directory string) (string, bool) {
	if directory == "~" {
		return "cd", true
	}
	if strings.ContainsAny(directory, shellSpecialCharacters) {
		return "", false
	}
	if underHome, ok := strings.CutPrefix(directory, "~/"); ok {
		return "cd -- ~/'" + underHome + "'", true
	}
	return "cd -- '" + directory + "'", true
}
