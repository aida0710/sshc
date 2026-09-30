package effective

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"strings"
)

// ErrUnknownToken は、ここで展開できないトークンを報告する。
//
// 展開できないものを暗黙に残すと、`%C` を含む IdentityFile が、そういう名前の
// ファイルを指しているかのように見える。応答しないことと、間違って返すことは
// 別である。
var ErrUnknownToken = errors.New("that token cannot be expanded here")

// LocalFacts は、このマシンについての事実。トークン展開と、設定に書かれていない
// ときに OpenSSH が読むファイルに要る。
//
// 注入するのは、テストが本物のホームディレクトリや /etc/ssh へ届かないように
// するためである。プロセスの性質であってワークスペースの性質ではないので、
// 設定からは読めない。
type LocalFacts struct {
	// User はローカルのアカウント名。%u。
	User string
	// Home はローカルのホームディレクトリ。%d。
	Home string
	// Hostname はローカルのホスト名。%L が最初のドットまで、%l が全体。
	Hostname string
	// UID はローカルの uid を十進で書いたもの。%i。
	UID string
	// LookupEnv は環境変数を引く。${NAME}。nil なら、どの変数にも値が無いものとして扱う。
	LookupEnv func(name string) (string, bool)
	// GlobalKnownHostsFiles は、GlobalKnownHostsFile が書かれていないときにホスト鍵の
	// 照合に読むファイル（/etc/ssh/ssh_known_hosts など）。空なら読まない。
	GlobalKnownHostsFiles []string
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
	// HostKeyAlias は HostKeyAlias の値（小文字）。%k。空なら Alias を使う。
	HostKeyAlias string
	// JumpHost は ProxyJump の最後のホップのホスト名。%j。ProxyJump が無ければ空。
	JumpHost string
}

// ExpandTokens は、OpenSSH が置き換えるトークンを置き換える。
//
// ここで扱わないもの（%f や %T のように接続の途中でしか意味を持たないもの）は
// ErrUnknownToken として拒む。展開できないまま返せば、その
// 文字列はファイル名やコマンドとしてそのまま使われてしまう。
func ExpandTokens(value string, facts LocalFacts, target TokenTarget) (string, error) {
	return tokenExpansion{facts: facts, target: target}.expand(value)
}

// expandVariablesAndTokens は、ExpandTokens に加えて、${NAME} を環境変数の値に置き換える。
//
// OpenSSH の misc.c の vdollar_percent_expand と同じく、value を左から 1 回だけ読む。
// 置き換えた値は読み直さないので、環境変数の値の中の % や ${ は文字どおりに残る。
func expandVariablesAndTokens(value string, facts LocalFacts, target TokenTarget) (string, error) {
	return tokenExpansion{facts: facts, target: target, variables: true}.expand(value)
}

// tokenExpansion は、値を左から 1 回だけ読んで % トークンを置き換える。variables が
// 真なら ${NAME} も置き換える。
type tokenExpansion struct {
	facts     LocalFacts
	target    TokenTarget
	variables bool
}

func (expansion tokenExpansion) expand(value string) (string, error) {
	var builder strings.Builder
	for index := 0; index < len(value); index++ {
		switch {
		case expansion.variables && strings.HasPrefix(value[index:], variablePrefix):
			replacement, length, err := expandVariable(value[index:], expansion.facts.LookupEnv)
			if err != nil {
				return "", err
			}
			builder.WriteString(replacement)
			index += length - 1
		case value[index] == '%':
			if index+1 >= len(value) {
				// 末尾の単独の % は OpenSSH も受け付けない。
				return "", ErrUnknownToken
			}
			index++
			replacement, ok := expandOne(value[index], expansion.facts, expansion.target)
			if !ok {
				return "", ErrUnknownToken
			}
			builder.WriteString(replacement)
		default:
			builder.WriteByte(value[index])
		}
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
	case 'k':
		// OpenSSH は HostKeyAlias が無ければ利用者が打った名前（%n）を使う。
		if target.HostKeyAlias != "" {
			return target.HostKeyAlias, true
		}
		return target.Alias, true
	case 'j':
		return target.JumpHost, true
	case 'C':
		return connectionHash(facts, target), true
	default:
		return "", false
	}
}

// connectionHash は %C である。OpenSSH の ssh_connection_hash と同じく、%l%h%p%r%j を
// つないだ文字列の SHA-1 を小文字の十六進で書く。
func connectionHash(facts LocalFacts, target TokenTarget) string {
	sum := sha1.Sum([]byte(facts.Hostname + target.HostName + target.Port + target.RemoteUser + target.JumpHost))
	return hex.EncodeToString(sum[:])
}
