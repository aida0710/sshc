package effective

import (
	"strings"
)

// Values は、ひとつの alias について OpenSSH が実際に使う設定。Resolve が
// 組み立て、sshclient が接続先を決めるときに読む。
//
// 形を ssh -G の出力に合わせてあるのは、差分テストが OpenSSH の答えを同じ型に
// 読んで突き合わせるためである。このアプリケーションは ssh -G を回さない。
// Keywords は小文字のキーワードを最初に現れた順に保ち、Entries は、identityfile の
// ように複数回現れうるキーワードのすべての値を保つ。
type Values struct {
	Keywords []string
	Entries  map[string][]string
}

// First は keyword の最初の値を返す。なければ空文字列。
func (v Values) First(keyword string) string {
	values := v.Entries[strings.ToLower(keyword)]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// All は keyword のすべての値を出力順で返す。
func (v Values) All(keyword string) []string { return v.Entries[strings.ToLower(keyword)] }
