package platform

import (
	"errors"
	"net"
	"regexp"
	"strings"
)

const (
	// MaxAliasLength は、このアプリケーションがコマンドラインに載せてよい Host alias
	// の長さに上限を設ける。
	MaxAliasLength = 64
	// MaxHostnameLength は DNS の上限。ssh-keyscan の対象はこれを超えてはならない。
	MaxHostnameLength = 255
)

var (
	ErrUnsafeAlias    = errors.New("alias contains characters this application refuses to pass to an external program")
	ErrUnsafeHostname = errors.New("hostname contains characters this application refuses to pass to an external program")
	ErrUnsafePort     = errors.New("port is outside the TCP range")
)

// safeAliasPattern は、OpenSSH が受け付ける範囲より意図的に狭くしてある。
//
// OpenSSH は、空白・引用符・'%' トークン・先頭の '-' を含む Host alias も平気で
// 読む。そうした alias はオプション（"-oProxyCommand=..."）になりうるし、コピー
// されたコマンドラインの意味を変えうるし、端末自動化のペイロード内で文字列から
// 抜け出しうる。この集合の外にある alias が起動されたり評価されたりすることは
// 決してない。UI は代わりに、コピー可能なテキストとしてそのコマンドを
// 提示する。
var safeAliasPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// safeHostnamePattern は DNS 名と IPv4 リテラルに使う。IPv6 は圧縮表記の先頭や
// 末尾が ':' になりうるため、文字集合の正規表現ではなく net.ParseIP へ渡す。
var safeHostnamePattern = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9._-]*[A-Za-z0-9])?$`)

// ValidateAlias は、alias を外部プログラムへ渡してよいかを報告する。
func ValidateAlias(alias string) error {
	if len(alias) == 0 || len(alias) > MaxAliasLength || !safeAliasPattern.MatchString(alias) {
		return ErrUnsafeAlias
	}
	return nil
}

// ValidateHostname は、host を外部プログラムへ渡してよいかを報告する。
func ValidateHostname(host string) error {
	if len(host) == 0 || len(host) > MaxHostnameLength {
		return ErrUnsafeHostname
	}
	if strings.Contains(host, ":") {
		// Brackets and zone identifiers are intentionally rejected. Callers add
		// brackets themselves where OpenSSH's host:port notation requires them.
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

// ValidatePort は、port が使用可能な TCP ポートかを報告する。
func ValidatePort(port int) error {
	if port < 1 || port > 65535 {
		return ErrUnsafePort
	}
	return nil
}
