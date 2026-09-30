package effective

import (
	"errors"
	"fmt"
	"strings"
)

// ErrEnvironmentVariable は、${NAME} を展開できないことを報告する。
//
// OpenSSH は、閉じない ${、名前の無い ${}、値の無い変数のどれでも接続をやめる。
// 空の文字列に置き換えて続けると、別のファイルを開くことになる。
var ErrEnvironmentVariable = errors.New("an environment variable cannot be expanded")

// variablePrefix は、環境変数の参照の始まりである。
const variablePrefix = "${"

// expandVariable は、text の先頭の ${NAME} を環境変数の値に置き換え、その値と、
// 読んだ ${NAME} のバイト数を返す。text は variablePrefix で始まっている。
//
// lookupEnv が nil なら、どの変数にも値が無いものとして扱う。
func expandVariable(text string, lookupEnv func(string) (string, bool)) (string, int, error) {
	name, _, closed := strings.Cut(text[len(variablePrefix):], "}")
	if !closed {
		return "", 0, fmt.Errorf("%w: ${ has no closing }", ErrEnvironmentVariable)
	}
	if name == "" {
		return "", 0, fmt.Errorf("%w: ${} has no name", ErrEnvironmentVariable)
	}
	found, ok := "", false
	if lookupEnv != nil {
		found, ok = lookupEnv(name)
	}
	if !ok {
		return "", 0, fmt.Errorf("%w: ${%s} is not set", ErrEnvironmentVariable, name)
	}
	return found, len(variablePrefix) + len(name) + len("}"), nil
}
