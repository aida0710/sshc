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
	// OpenConnect は openconnect でトンネルを張る。Cisco AnyConnect と
	// その仲間（ocserv、GlobalProtect、Pulse など）へ繋ぐ。
	OpenConnect BackendName = "openconnect"
)

// openConnectProtocols は、openconnect が話せる方式のうち、このプロファイルで
// 指定できるものである。openconnect の --protocol にそのまま渡る。
var openConnectProtocols = map[string]bool{
	"anyconnect": true, "nc": true, "pulse": true, "gp": true,
	"f5": true, "fortinet": true, "array": true,
}

// パスワードのあとに装置がすることへの備えである。
const (
	// SecondFactorApprove は、利用者が電話で承認するのを待つ。Duo Mobile などが
	// 通知を出し、承認するまで装置は応答を返さない。
	//
	// 二段目を聞いてくる装置と、何も聞かずに通知だけ出す装置がある。前者には
	// ApprovalWord を送り、後者には何も送らない。どちらも、待つ長さが普通の
	// 接続より長いことは同じである。
	SecondFactorApprove = "approve"
	// SecondFactorTOTP は、Vault に置いた種から作ったコードを送る。
	SecondFactorTOTP = "totp"
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
	// ErrTargetMismatch は、繋ごうとしている相手と、その経路の接続先が食い違う
	// ことを表す。
	//
	// コンテナはプロファイルの接続先ひとつだけを通す。食い違ったまま繋ぐと、
	// 利用者が設定に書いた相手ではなく、プロファイルに書いた相手へ届く。どちらが
	// 正しいかを推測せず、断る。
	ErrTargetMismatch = errors.New("the connection and its vpn profile name different targets")
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
	DNS         []string
	WireGuard   *WireGuardSettings
	L2TP        *L2TPSettings
	OpenConnect *OpenConnectSettings
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

// OpenConnectSettings は、openconnect backendの秘密でない設定である。
type OpenConnectSettings struct {
	// Server は、VPN装置の名前またはアドレスである。名前はコンテナの中で引く。
	Server string
	// Username は、VPNの利用者名である。パスワードはVaultにある。
	Username string
	// Protocol は、その装置が話す方式である。空なら anyconnect。
	Protocol string
	// ServerCertificate は、相手の証明書を固定する指紋である（`sha256:...`）。
	// 公的な認証局の証明書を使う装置では空でよい。自己署名の装置では、これが
	// 無いと openconnect は繋がない。
	ServerCertificate string
	// SecondFactor は、パスワードのあとへの備えである。空なら何もしない。
	SecondFactor string
	// ApprovalWord は、SecondFactor が approve のときに二段目へ送る語である。
	// 空なら何も送らない。装置が二段目を聞いてくる場合だけ書く（Duo なら
	// push、装置によっては phone や sms）。
	ApprovalWord string
}

// Secrets は、プロファイルの秘密である。Vaultから読み、標準入力でコンテナへ渡す。
type Secrets struct {
	WireGuardPrivateKey string
	// L2TPPassword は、VPNの利用者のパスワードである。
	L2TPPassword string
	// IPsecPSK は、IPsecの事前共有鍵である。
	IPsecPSK string
	// OpenConnectPassword は、openconnect backend の利用者のパスワードである。
	OpenConnectPassword string
	// OpenConnectTOTPSecret は、二段目のコードを作る種である（base32 または
	// otpauth URI）。SecondFactor が totp のときだけ使う。
	OpenConnectTOTPSecret string
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
	case OpenConnect:
		if profile.OpenConnect == nil {
			return fmt.Errorf("%w: openconnect settings are absent", ErrSettings)
		}
		return profile.OpenConnect.validate()
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
	case OpenConnect:
		if secrets.OpenConnectPassword == "" {
			return fmt.Errorf("%w: VPNのパスワードがありません", ErrSecrets)
		}
		if profile.OpenConnect != nil && profile.OpenConnect.SecondFactor == SecondFactorTOTP &&
			secrets.OpenConnectTOTPSecret == "" {
			return fmt.Errorf("%w: 二段目のTOTPの種がありません", ErrSecrets)
		}
		return nil
	}
	return fmt.Errorf("%w: %s", ErrBackend, profile.Backend)
}

func (settings OpenConnectSettings) validate() error {
	if settings.Server == "" || strings.ContainsAny(settings.Server, " \t\n\"\\") {
		return fmt.Errorf("%w: VPN装置の指定が使えません", ErrSettings)
	}
	if settings.Username == "" || strings.ContainsAny(settings.Username, "\n\"\\") {
		return fmt.Errorf("%w: VPNの利用者名が使えません", ErrSettings)
	}
	if settings.Protocol != "" && !openConnectProtocols[settings.Protocol] {
		return fmt.Errorf("%w: 方式 %q は openconnect が知りません", ErrSettings, settings.Protocol)
	}
	if settings.ServerCertificate != "" &&
		!strings.HasPrefix(settings.ServerCertificate, "sha256:") &&
		!strings.HasPrefix(settings.ServerCertificate, "pin-sha256:") {
		// openconnect が受け取るのはこの2つの書き方である。証明書そのものの
		// SHA-256（`sha256:`、16進）と、公開鍵のPIN（`pin-sha256:`、base64）。
		return fmt.Errorf("%w: 相手の証明書の指紋は sha256: か pin-sha256: で始まります", ErrSettings)
	}
	if strings.ContainsAny(settings.ServerCertificate, " \t\n\"\\") {
		return fmt.Errorf("%w: 相手の証明書の指紋が使えません", ErrSettings)
	}
	switch settings.SecondFactor {
	case "", SecondFactorApprove, SecondFactorTOTP:
	default:
		return fmt.Errorf("%w: 二段目の答え方 %q は知りません", ErrSettings, settings.SecondFactor)
	}
	// 答えは1行として送る。改行が混じると、装置が受け取る問答がずれる。
	if strings.ContainsAny(settings.ApprovalWord, " \t\r\n") {
		return fmt.Errorf("%w: 承認の合図に空白や改行は使えません", ErrSettings)
	}
	return nil
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
