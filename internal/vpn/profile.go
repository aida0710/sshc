// Package vpn は、ひとつのSSH接続だけを専用のVPNへ通す経路を用意する。
//
// ホストの既定経路もDNSも変えない。VPNはプロファイルごとのコンテナの中だけに
// 存在し、engineはそのコンテナが差し出すUnixソケットへ繋ぐ。SSHの握手・認証・
// ホスト鍵の照合はengineが行うので、鍵もVaultもコンテナへは渡らない。
package vpn

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"reflect"
	"strings"
)

// BackendName は、トンネルの張り方である。
type BackendName string

const (
	// WireGuard は userspace の wireguard-go でトンネルを張る。
	WireGuard BackendName = "wireguard"
	// L2TPIPsec は strongSwan と xl2tpd と pppd でトンネルを張る。大学や
	// 会社の装置に多い方式である。
	L2TPIPsec BackendName = "l2tp_ipsec"
)

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

// maxProfileNameLength は、コンテナ名とソケットのパスに入る長さに収める。
const maxProfileNameLength = 48

// Endpoint は、host と port の組である。
type Endpoint struct {
	Host string
	Port int
}

// Address は、接続先の表記を返す。
func (endpoint Endpoint) Address() string {
	return net.JoinHostPort(endpoint.Host, fmt.Sprint(endpoint.Port))
}

// Profile は、ひとつのVPN経路である。
//
// 接続先はひとつに限る。VPNの向こうのネットワーク全体を引き込まないので、経路と
// パケットフィルタが接続先ひとつで閉じ、取り違える余地が残らない。
type Profile struct {
	Name    string
	Backend BackendName
	// Target は、このVPNの中にある接続先である。IPv4アドレスか、VPNの中の
	// DNSで引ける名前を書く。名前で書くときは DNS が要る。
	Target Endpoint
	// DNS は、VPNの中で名前を引くDNSサーバーである（IPv4）。この経路の中
	// だけで使い、ホストのDNSもコンテナの既定のDNSも変えない。
	DNS       []string
	WireGuard *WireGuardSettings
	L2TP      *L2TPSettings
}

// maxResolvers は、1つの経路が使うDNSサーバーの数の上限である。VPNの中の名前
// ひとつを引くためのもので、並べるほど引ける名前が増えるわけではない。
const maxResolvers = 3

// WireGuardSettings は、wireguard backendの秘密でない設定である。
type WireGuardSettings struct {
	// Server は、トンネルの相手である。ここへはコンテナの通常回線で届く。
	Server Endpoint
	// PeerPublicKey は、相手の公開鍵である。秘密ではない。
	PeerPublicKey string
	// Address は、トンネル側でこの端末が名乗るアドレスである（CIDR表記）。
	Address string
}

// L2TPSettings は、l2tp_ipsec backendの秘密でない設定である。
type L2TPSettings struct {
	// Server は、VPN装置の名前またはアドレスである。名前はコンテナの中で
	// 引く。IPsecは相手のアドレスを設定に書くので、引く場所が違えば別の装置へ
	// 繋ぎうる。
	Server string
	// Username は、VPNの利用者名である。パスワードはVaultにある。
	Username string
	// IKE と ESP は、古い装置と暗号方式が合わないときだけ指定する。空なら
	// strongSwan の既定に任せる。
	IKE string
	ESP string
}

// Secrets は、プロファイルの秘密である。Vaultから読み、標準入力でコンテナへ渡す。
type Secrets struct {
	WireGuardPrivateKey string
	// L2TPPassword は、VPNの利用者のパスワードである。
	L2TPPassword string
	// IPsecPSK は、IPsecの事前共有鍵である。
	IPsecPSK string
}

// sameRouteAs は、この設定がもう一方と同じ経路を作るかを返す。
//
// 設定が変わったコンテナは作り直す。古い設定のまま繋ぎ続けると、利用者が直した
// 先へ行かない。
func (profile Profile) sameRouteAs(other Profile) bool {
	return reflect.DeepEqual(profile, other)
}

// Validate は、このプロファイルで経路を作れるかを確かめる。
func (profile Profile) Validate() error {
	if err := validateProfileName(profile.Name); err != nil {
		return err
	}
	if err := validateResolvers(profile.DNS); err != nil {
		return err
	}
	if err := validateTarget(profile.Target, profile.DNS); err != nil {
		return err
	}
	switch profile.Backend {
	case WireGuard:
		if profile.WireGuard == nil {
			return fmt.Errorf("%w: wireguard settings are absent", ErrSettings)
		}
		return profile.WireGuard.validate()
	case L2TPIPsec:
		if profile.L2TP == nil {
			return fmt.Errorf("%w: l2tp settings are absent", ErrSettings)
		}
		return profile.L2TP.validate()
	}
	return fmt.Errorf("%w: %s", ErrBackend, profile.Backend)
}

// ValidateName は、プロファイル名として使えるかを確かめる。
//
// 保存する側も同じ規則で確かめる。コンテナ名とディレクトリ名になるので、
// 区切り文字が混じったものを保存させない。
func ValidateName(name string) error { return validateProfileName(name) }

// validateProfileName は、コンテナ名とディレクトリ名に入る字だけを通す。
func validateProfileName(name string) error {
	if name == "" || len(name) > maxProfileNameLength {
		return fmt.Errorf("%w: %q", ErrProfileName, name)
	}
	for _, character := range name {
		letter := character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z'
		digit := character >= '0' && character <= '9'
		if !letter && !digit && character != '-' && character != '_' {
			return fmt.Errorf("%w: %q", ErrProfileName, name)
		}
	}
	return nil
}

// validateTarget は、接続先がIPv4アドレスか、引ける名前であることを確かめる。
//
// 名前はコンテナの中で、この経路のDNSだけを使って引く。どちらの名前空間で引く
// のかが決まらないまま名前を許すと、ホストで引いた別の機械へ繋ぎうる。だから
// 名前で書くときはDNSを必ず添えさせる。
func validateTarget(target Endpoint, resolvers []string) error {
	if target.Port <= 0 || target.Port > 65535 {
		return fmt.Errorf("%w: ポート %d", ErrTarget, target.Port)
	}
	address, err := netip.ParseAddr(target.Host)
	if err != nil {
		if len(resolvers) == 0 {
			return fmt.Errorf("%w: %q を名前で書くには、VPNの中のDNSサーバーが要ります", ErrTarget, target.Host)
		}
		return validateHostName(target.Host)
	}
	if !address.Is4() {
		return fmt.Errorf("%w: %q はIPv4アドレスではありません", ErrTarget, target.Host)
	}
	if address.IsUnspecified() || address.IsLoopback() || address.IsMulticast() {
		return fmt.Errorf("%w: %q へは経路を作れません", ErrTarget, target.Host)
	}
	return nil
}

// validateHostName は、コンテナの中でそのまま引ける名前だけを通す。
//
// この名前は agent が sh の変数として扱う。区切り文字や空白が混じったものを
// 渡すと、名前を引く以外のことが起こりうる。
func validateHostName(name string) error {
	if name == "" || len(name) > 253 {
		return fmt.Errorf("%w: %q は接続先の名前として使えません", ErrTarget, name)
	}
	for _, label := range strings.Split(strings.TrimSuffix(name, "."), ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return fmt.Errorf("%w: %q は接続先の名前として使えません", ErrTarget, name)
		}
		for _, character := range label {
			letter := character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z'
			digit := character >= '0' && character <= '9'
			if !letter && !digit && character != '-' {
				return fmt.Errorf("%w: %q は接続先の名前として使えません", ErrTarget, name)
			}
		}
	}
	return nil
}

// validateResolvers は、VPNの中のDNSサーバーがIPv4アドレスであることを確かめる。
func validateResolvers(resolvers []string) error {
	if len(resolvers) > maxResolvers {
		return fmt.Errorf("%w: DNSサーバーは%d件までです", ErrTarget, maxResolvers)
	}
	for _, resolver := range resolvers {
		address, err := netip.ParseAddr(resolver)
		if err != nil || !address.Is4() {
			return fmt.Errorf("%w: DNSサーバー %q はIPv4アドレスではありません", ErrTarget, resolver)
		}
		if address.IsUnspecified() || address.IsLoopback() || address.IsMulticast() {
			return fmt.Errorf("%w: DNSサーバー %q へは経路を作れません", ErrTarget, resolver)
		}
	}
	return nil
}

func (settings WireGuardSettings) validate() error {
	if settings.Server.Host == "" || strings.ContainsAny(settings.Server.Host, " \t\n") {
		return fmt.Errorf("%w: wireguardのサーバーが指定されていません", ErrSettings)
	}
	if settings.Server.Port <= 0 || settings.Server.Port > 65535 {
		return fmt.Errorf("%w: wireguardのポート %d", ErrSettings, settings.Server.Port)
	}
	if err := validateWireGuardKey(settings.PeerPublicKey, "相手の公開鍵"); err != nil {
		return err
	}
	prefix, err := netip.ParsePrefix(settings.Address)
	if err != nil || !prefix.Addr().Is4() {
		return fmt.Errorf("%w: トンネル側アドレス %q はIPv4のCIDR表記ではありません", ErrSettings, settings.Address)
	}
	return nil
}

// validateWireGuardKey は、鍵がbase64の32バイトであることだけを確かめる。
//
// 設定ファイルへ書く値なので、改行や引用符が混じったまま渡さない。
func validateWireGuardKey(key, label string) error {
	if len(key) != 44 || !strings.HasSuffix(key, "=") {
		return fmt.Errorf("%w: %sの形式が違います", ErrSettings, label)
	}
	for _, character := range key[:len(key)-1] {
		letter := character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z'
		digit := character >= '0' && character <= '9'
		if !letter && !digit && character != '+' && character != '/' {
			return fmt.Errorf("%w: %sの形式が違います", ErrSettings, label)
		}
	}
	return nil
}

// ValidateSecrets は、このbackendが要る秘密が揃っているかを確かめる。
func (profile Profile) ValidateSecrets(secrets Secrets) error {
	switch profile.Backend {
	case WireGuard:
		if secrets.WireGuardPrivateKey == "" {
			return fmt.Errorf("%w: wireguardの秘密鍵がありません", ErrSecrets)
		}
		return validateWireGuardKey(secrets.WireGuardPrivateKey, "秘密鍵")
	case L2TPIPsec:
		if secrets.L2TPPassword == "" {
			return fmt.Errorf("%w: VPNのパスワードがありません", ErrSecrets)
		}
		if secrets.IPsecPSK == "" {
			return fmt.Errorf("%w: IPsecの事前共有鍵がありません", ErrSecrets)
		}
		return nil
	}
	return fmt.Errorf("%w: %s", ErrBackend, profile.Backend)
}

func (settings L2TPSettings) validate() error {
	if settings.Server == "" || strings.ContainsAny(settings.Server, " \t\n\"") {
		return fmt.Errorf("%w: VPN装置の指定が使えません", ErrSettings)
	}
	if settings.Username == "" || strings.ContainsAny(settings.Username, "\n\"\\") {
		return fmt.Errorf("%w: VPNの利用者名が使えません", ErrSettings)
	}
	for name, proposal := range map[string]string{"ike": settings.IKE, "esp": settings.ESP} {
		if proposal != "" && strings.ContainsAny(proposal, " \n\"\\") {
			return fmt.Errorf("%w: %s の指定が使えません", ErrSettings, name)
		}
	}
	return nil
}
