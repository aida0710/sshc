package validate

import (
	"regexp"
	"strconv"
	"strings"
)

const (
	// DirectUserPattern は、保存していない接続先のユーザー名が従う形である。
	//
	// ユーザー名は Host * の ProxyCommand などの %r に入り、シェルが読む。設定ファイルの User と
	// 違って手で打った値なので、OpenSSH がコマンドラインのユーザー名で断る文字（valid_ruser の
	// '`";&<>|(){}）と先頭の `-` を断る。`-` で始まる名前はコマンドのオプションになりうる。
	// シェルが読むと展開や区切りになる `$`、`\`、空白、制御文字も断る。
	DirectUserPattern = "^[^\\x00-\\x20\\x7f'\"`$\\\\;&<>|(){}-][^\\x00-\\x20\\x7f'\"`$\\\\;&<>|(){}]*$"
	// MaxUserLength は、ユーザー名の長さの上限である（API の CreateConnectionRequest と同じ）。
	MaxUserLength = 255
	// MaxDirectDestinationLength は、user@[host]:port の形の長さの上限である。
	MaxDirectDestinationLength = MaxUserLength + len("@[") + MaxHostnameLength + len("]:65535")
)

var directUserPattern = regexp.MustCompile(DirectUserPattern)

// DirectDestination は、設定ファイルに保存せずに接続する接続先である。ssh コマンドの
// user@host[:port] と同じ形で書く。接続名は @ を含まないので（AliasPattern）、接続名と
// 取り違えることはない。
type DirectDestination struct {
	User string
	Host string
	// Port が 0 なら、ポートは書かれていない。設定ファイルの値か既定の 22 を使う。
	Port int
}

// ParseDirectDestination は、text が保存していない接続先の形なら読んで返す。
// ユーザー名に @ を含められるように（user@domain@host）、最後の @ で分ける。
func ParseDirectDestination(text string) (DirectDestination, bool) {
	at := strings.LastIndex(text, "@")
	if at < 0 || len(text) > MaxDirectDestinationLength {
		return DirectDestination{}, false
	}
	user := text[:at]
	if len(user) > MaxUserLength || !directUserPattern.MatchString(user) {
		return DirectDestination{}, false
	}
	host, port, ok := splitDirectHost(text[at+1:])
	if !ok || Hostname(host) != nil {
		return DirectDestination{}, false
	}
	return DirectDestination{User: user, Host: host, Port: port}, true
}

// splitDirectHost は、host[:port] を分ける。角括弧の無い IPv6 アドレス（コロンが2つ以上）には
// ポートを書けない。ssh の接続先と同じである。
func splitDirectHost(text string) (host string, port int, ok bool) {
	portText, hasPort := "", false
	switch {
	case strings.HasPrefix(text, "["):
		closing := strings.Index(text, "]")
		if closing < 0 {
			return "", 0, false
		}
		host = text[1:closing]
		remainder := text[closing+1:]
		if remainder != "" && !strings.HasPrefix(remainder, ":") {
			return "", 0, false
		}
		portText, hasPort = strings.CutPrefix(remainder, ":")
	case strings.Count(text, ":") > 1:
		host = text
	default:
		host, portText, hasPort = strings.Cut(text, ":")
	}
	if !hasPort {
		return host, 0, true
	}
	parsed, err := strconv.ParseUint(portText, 10, 16)
	if err != nil || Port(int(parsed)) != nil {
		return "", 0, false
	}
	return host, int(parsed), true
}

// SessionTarget は、ターミナルのセッションを開く先として受け付けてよいかを報告する。
// 接続名か、保存していない接続先である。
func SessionTarget(target string) error {
	if _, direct := ParseDirectDestination(target); direct {
		return nil
	}
	return Alias(target)
}
