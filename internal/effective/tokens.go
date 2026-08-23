package effective

import (
	"errors"
	"strings"
)

// ErrUnknownToken は、ここで展開できないトークンを報告する。
//
// 展開できないものを暗黙に残すと、`%C` を含む IdentityFile が、そういう名前の
// ファイルを指しているかのように見える。応答しないことと、間違って返すことは
// 別である。
var ErrUnknownToken = errors.New("that token cannot be expanded here")

// LocalFacts は、このマシンについての事実。トークン展開に要る。
//
// 注入するのは、テストが本物のホームディレクトリへ届かないようにするためである。
// プロセスの性質であってワークスペースの性質ではないので、設定からは読めない。
type LocalFacts struct {
	// User はローカルのアカウント名。%u。
	User string
	// Home はローカルのホームディレクトリ。%d。
	Home string
	// Hostname はローカルのホスト名。%L が最初のドットまで、%l が全体。
	Hostname string
	// UID はローカルの uid を十進で書いたもの。%i。
	UID string
}

// TokenTarget は、接続先について決まった事実。
//
// HostName と Port と RemoteUser は解決の結果なので、展開は走査のあとに一度だけ
// 行う。走査しながら展開すると、まだ決まっていない HostName で %h を潰すことになる。
type TokenTarget struct {
	// Alias は利用者が打った名前。%n。
	Alias string
	// HostName は解決後の接続先。%h。
	HostName string
	// Port は解決後のポート。%p。
	Port string
	// RemoteUser は解決後のリモートのアカウント名。%r。
	RemoteUser string
}

// ExpandTokens は、OpenSSH が置き換えるトークンを置き換える。
//
// ここで扱わないもの（%C のハッシュ、%f や %T のように接続の途中でしか意味を
// 持たないもの）は ErrUnknownToken として拒む。展開できないまま返せば、その
// 文字列はファイル名やコマンドとしてそのまま使われてしまう。
func ExpandTokens(value string, facts LocalFacts, target TokenTarget) (string, error) {
	if !strings.ContainsRune(value, '%') {
		return value, nil
	}
	var builder strings.Builder
	for index := 0; index < len(value); index++ {
		if value[index] != '%' {
			builder.WriteByte(value[index])
			continue
		}
		if index+1 >= len(value) {
			// 末尾の単独の % は OpenSSH も受け付けない。
			return "", ErrUnknownToken
		}
		index++
		replacement, ok := expandOne(value[index], facts, target)
		if !ok {
			return "", ErrUnknownToken
		}
		builder.WriteString(replacement)
	}
	return builder.String(), nil
}

func expandOne(token byte, facts LocalFacts, target TokenTarget) (string, bool) {
	switch token {
	case '%':
		return "%", true
	case 'h':
		return target.HostName, true
	case 'n':
		return target.Alias, true
	case 'p':
		return target.Port, true
	case 'r':
		return target.RemoteUser, true
	case 'u':
		return facts.User, true
	case 'd':
		return facts.Home, true
	case 'i':
		return facts.UID, true
	case 'l':
		return facts.Hostname, true
	case 'L':
		// %L は最初のドットまで。ドットが無ければ %l と同じものになる。
		if cut, _, found := strings.Cut(facts.Hostname, "."); found {
			return cut, true
		}
		return facts.Hostname, true
	default:
		return "", false
	}
}
