package vpn

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
)

// 設定と秘密を受け取れない理由を、項目と理由の組で表す。
//
// 文言は Go に持たない。画面と CLI が、理由の語を自分の言葉へ翻訳する。項目の
// 名前は、保存形式（metadata.json と API）の JSON のパスである。

var (
	// ErrProfileName は、プロファイル名が使えないことを表す。
	ErrProfileName = errors.New("vpn profile name is invalid")
	// ErrBackend は、知らないbackendを拒む。
	ErrBackend = errors.New("vpn backend is not supported")
	// ErrTarget は、接続先の指定が使えないことを表す。
	ErrTarget = errors.New("vpn target is invalid")
	// ErrSettings は、backend固有の設定が足りないか、形式が違うことを表す。
	ErrSettings = errors.New("vpn settings are invalid")
	// ErrSecrets は、backendが要る秘密を受け取れなかったことを表す。
	ErrSecrets = errors.New("vpn secrets are missing")
)

// Reason は、値を受け取れない理由である。
type Reason string

const (
	// ReasonRequired は、値が無いことを表す。
	ReasonRequired Reason = "required"
	// ReasonFormat は、書き方が違うことを表す。
	ReasonFormat Reason = "format"
	// ReasonTooLong は、長すぎることを表す。Limit に上限が入る。
	ReasonTooLong Reason = "too_long"
	// ReasonTooMany は、数が多すぎることを表す。Limit に上限が入る。
	ReasonTooMany Reason = "too_many"
	// ReasonOutOfRange は、ポート番号が範囲の外にあることを表す。
	ReasonOutOfRange Reason = "out_of_range"
	// ReasonNotIPv4 は、IPv4 アドレスでないことを表す。
	ReasonNotIPv4 Reason = "not_ipv4"
	// ReasonUnroutable は、経路を作れないアドレス（ループバックなど）であることを表す。
	ReasonUnroutable Reason = "unroutable"
	// ReasonNameNeedsDNS は、接続先を名前で書いたのに VPN の中の DNS が無いことを表す。
	ReasonNameNeedsDNS Reason = "name_needs_dns"
	// ReasonUnsupported は、知らない値（backend、方式、二段目の答え方）であることを表す。
	ReasonUnsupported Reason = "unsupported"
	// ReasonUnexpected は、選んだ backend と違う backend の節があることを表す。
	ReasonUnexpected Reason = "unexpected"
)

// FieldError は、ひとつの項目を受け取れない理由である。
type FieldError struct {
	// Kind は、errors.Is で見分けるための分類（ErrTarget など）である。
	Kind error
	// Field は、保存形式での項目の JSON パスである（例: `wireguard.server`）。
	Field  string
	Reason Reason
	// Limit は、ReasonTooLong と ReasonTooMany のときの上限である。
	Limit int
}

func (failure *FieldError) Error() string {
	if failure.Limit > 0 {
		return fmt.Sprintf("%v: %s: %s (limit %d)", failure.Kind, failure.Field, failure.Reason, failure.Limit)
	}
	return fmt.Sprintf("%v: %s: %s", failure.Kind, failure.Field, failure.Reason)
}

func (failure *FieldError) Unwrap() error { return failure.Kind }

func fieldError(kind error, field string, reason Reason) *FieldError {
	return &FieldError{Kind: kind, Field: field, Reason: reason}
}

const (
	// maxProfileNameLength は、コンテナ名とソケットのパスに入る長さに収める。
	maxProfileNameLength = 48
	// maxResolvers は、1つの経路が使うDNSサーバーの数の上限である。VPNの中の名前
	// ひとつを引くためのもので、並べるほど引ける名前が増えるわけではない。
	maxResolvers = 3
	// maxHostNameLength と maxHostLabelLength は、DNS の名前の上限である（RFC 1035）。
	maxHostNameLength  = 253
	maxHostLabelLength = 63
)

// ValidateName は、プロファイル名として使えるかを確かめる。
//
// 保存する側も同じ規則で確かめる。コンテナ名とディレクトリ名になるので、
// 区切り文字が混じったものを保存させない。
func ValidateName(name string) error { return validateProfileName(name) }

// validateProfileName は、コンテナ名とディレクトリ名に入る字だけを通す。
func validateProfileName(name string) error {
	if name == "" {
		return fieldError(ErrProfileName, "name", ReasonRequired)
	}
	if len(name) > maxProfileNameLength {
		return &FieldError{Kind: ErrProfileName, Field: "name", Reason: ReasonTooLong, Limit: maxProfileNameLength}
	}
	for _, character := range name {
		if !isASCIIAlphanumeric(character) && character != '-' && character != '_' {
			return fieldError(ErrProfileName, "name", ReasonFormat)
		}
	}
	return nil
}

// isASCIIAlphanumeric は、英字か数字かを返す。名前・鍵・DNSの名前の検査が使う。
func isASCIIAlphanumeric(character rune) bool {
	return character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
		character >= '0' && character <= '9'
}

// validatePort は、TCP/UDP のポート番号として使えるかを確かめる。
func validatePort(kind error, field string, port int) error {
	if port <= 0 || port > 65535 {
		return fieldError(kind, field, ReasonOutOfRange)
	}
	return nil
}

// validateTarget は、接続先がIPv4アドレスか、引ける名前であることを確かめる。
//
// 名前はコンテナの中で、この経路のDNSだけを使って引く。どちらの名前空間で引く
// のかが決まらないまま名前を許すと、ホストで引いた別の機械へ繋ぎうる。だから
// 名前で書くときはDNSを必ず添えさせる。
func validateTarget(target Endpoint, resolvers []string) error {
	if err := validatePort(ErrTarget, "target", target.Port); err != nil {
		return err
	}
	if target.Host == "" {
		return fieldError(ErrTarget, "target", ReasonRequired)
	}
	address, err := netip.ParseAddr(target.Host)
	if err != nil {
		if !validHostName(target.Host) {
			return fieldError(ErrTarget, "target", ReasonFormat)
		}
		if len(resolvers) == 0 {
			return fieldError(ErrTarget, "target", ReasonNameNeedsDNS)
		}
		return nil
	}
	return validateRoutableIPv4(ErrTarget, "target", address)
}

// validateRoutableIPv4 は、コンテナの中で /32 の経路を作れる IPv4 アドレスかを確かめる。
func validateRoutableIPv4(kind error, field string, address netip.Addr) error {
	if !address.Is4() {
		return fieldError(kind, field, ReasonNotIPv4)
	}
	if address.IsUnspecified() || address.IsLoopback() || address.IsMulticast() {
		return fieldError(kind, field, ReasonUnroutable)
	}
	return nil
}

// validHostName は、コンテナの中でそのまま引ける名前かを返す。
//
// この名前は agent が sh の変数として扱う。区切り文字や空白が混じったものを
// 渡すと、名前を引く以外のことが起こりうる。
func validHostName(name string) bool {
	if name == "" || len(name) > maxHostNameLength {
		return false
	}
	for _, label := range strings.Split(strings.TrimSuffix(name, "."), ".") {
		if label == "" || len(label) > maxHostLabelLength || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, character := range label {
			if !isASCIIAlphanumeric(character) && character != '-' {
				return false
			}
		}
	}
	return true
}

// validateResolvers は、VPNの中のDNSサーバーがIPv4アドレスであることを確かめる。
func validateResolvers(resolvers []string) error {
	if len(resolvers) > maxResolvers {
		return &FieldError{Kind: ErrSettings, Field: "dns", Reason: ReasonTooMany, Limit: maxResolvers}
	}
	for _, resolver := range resolvers {
		address, err := netip.ParseAddr(resolver)
		if err != nil {
			return fieldError(ErrSettings, "dns", ReasonNotIPv4)
		}
		if err := validateRoutableIPv4(ErrSettings, "dns", address); err != nil {
			return err
		}
	}
	return nil
}

// validateServerName は、VPN 装置の名前またはアドレスとして、設定ファイルと
// コマンド引数へそのまま書けるかを確かめる。
func validateServerName(field, server string) error {
	if server == "" {
		return fieldError(ErrSettings, field, ReasonRequired)
	}
	if strings.ContainsAny(server, " \t\r\n\"\\") {
		return fieldError(ErrSettings, field, ReasonFormat)
	}
	return nil
}

// validateUsername は、VPN の利用者名として設定ファイルへ1語で書けるかを確かめる。
func validateUsername(field, username string) error {
	if username == "" {
		return fieldError(ErrSettings, field, ReasonRequired)
	}
	if strings.ContainsAny(username, "\r\n\"\\") {
		return fieldError(ErrSettings, field, ReasonFormat)
	}
	return nil
}

// requireSecret は、backend が要る秘密があることを確かめる。
func requireSecret(field, value string) error {
	if value == "" {
		return fieldError(ErrSecrets, field, ReasonRequired)
	}
	return nil
}
