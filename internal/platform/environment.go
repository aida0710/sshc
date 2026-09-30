package platform

import (
	"runtime"
	"strings"
)

// environmentNamesFoldCase は、exec が環境変数の名前の大文字小文字を区別しない OS
// かどうかである。os/exec の重複除去は Windows でだけ名前を区別しない。
const environmentNamesFoldCase = runtime.GOOS == "windows"

// LookupEnvironment は、environment を exec.Cmd.Env に渡したときに、子プロセスが
// name として受け取る値を返す。規則は os/exec の重複除去に合わせる。同じ名前が
// 何度あっても後に書かれたものが勝ち、Windows では "Path" も "PATH" として読む。
func LookupEnvironment(environment []string, name string) (string, bool) {
	for index := len(environment) - 1; index >= 0; index-- {
		entry := environment[index]
		if len(entry) <= len(name) || entry[len(name)] != '=' {
			continue
		}
		if sameEnvironmentName(entry[:len(name)], name) {
			return entry[len(name)+1:], true
		}
	}
	return "", false
}

func sameEnvironmentName(left, right string) bool {
	if environmentNamesFoldCase {
		return strings.EqualFold(left, right)
	}
	return left == right
}
