// Package validate は、このアプリケーションが受け付ける名前と番号の規則を持つ。
//
// **OS を知らない。ディスクも触らない。ネットワークも引かない。** ここにあるのは
// 文字集合と長さの判断だけである。以前この規則は internal/platform に住んでいたが、
// あそこは「OS ごとに実装が変わるもの」の置き場であり、regexp と長さの比較はそれに
// あたらない。実際、IPv6 のホスト名を許すという変更が OS 抽象パッケージの diff に
// なっていた。
//
// **同じ規則が二箇所にあってはならない。** internal/application にも同じ文字集合を
// 手で書いた alias の検査があり、受理する集合は同じでありながら、返すエラーだけが
// 違っていた。規則はここにひとつ置き、エラーは各層が自分の語彙で包む。
package validate

import (
	"errors"
	"net"
	"regexp"
	"strings"
)

const (
	// MaxAliasLength は、このアプリケーションが受け付ける Host alias の長さの上限。
	MaxAliasLength = 64
	// MaxHostnameLength は DNS の上限。ssh-keyscan の対象はこれを超えてはならない。
	MaxHostnameLength = 255
)

var (
	ErrUnsafeAlias    = errors.New("alias contains characters this application refuses to accept")
	ErrUnsafeHostname = errors.New("hostname contains characters this application refuses to accept")
	ErrUnsafePort     = errors.New("port is outside the TCP range")
	// ErrInvalidGroupName は、connections ディレクトリ配下の安全な相対ディレクトリ
	// パスになっていないグループ名を報告する。
	ErrInvalidGroupName = errors.New("group name is not a safe relative directory path")
)

// safeAliasPattern は、OpenSSH が受け付ける範囲より意図的に狭くしてある。
//
// OpenSSH は、空白・引用符・'%' トークン・先頭の '-' を含む Host alias も平気で
// 読む。そうした alias はオプション（"-oProxyCommand=..."）になりうるし、コピー
// されたコマンドラインの意味を変えうるし、端末自動化のペイロード内で文字列から
// 抜け出しうる。この集合の外にある alias が起動されたり評価されたりすることは
// 決してない。UI は代わりに、コピー可能なテキストとしてそのコマンドを提示する。
var safeAliasPattern = regexp.MustCompile(AliasPattern)

// safeHostnamePattern は DNS 名と IPv4 リテラルに使う。IPv6 は圧縮表記の先頭や
// 末尾が ':' になりうるため、文字集合の正規表現ではなく net.ParseIP へ渡す。
var safeHostnamePattern = regexp.MustCompile(HostnamePattern)

// Alias は、この alias を受け付けてよいかを報告する。
func Alias(alias string) error {
	if len(alias) == 0 || len(alias) > MaxAliasLength || !safeAliasPattern.MatchString(alias) {
		return ErrUnsafeAlias
	}
	return nil
}

// Hostname は、この host を受け付けてよいかを報告する。
func Hostname(host string) error {
	if len(host) == 0 || len(host) > MaxHostnameLength {
		return ErrUnsafeHostname
	}
	if strings.Contains(host, ":") {
		// 角括弧と zone identifier は意図して拒む。OpenSSH の host:port 記法が
		// 括弧を要求する場所では、呼び出し側が自分で付ける。
		if net.ParseIP(host) == nil {
			return ErrUnsafeHostname
		}
		return nil
	}
	if !safeHostnamePattern.MatchString(host) {
		return ErrUnsafeHostname
	}
	return nil
}

// Port は、この番号が使用可能な TCP ポートかを報告する。
func Port(port int) error {
	if port < 1 || port > 65535 {
		return ErrUnsafePort
	}
	return nil
}
