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
	"reflect"
	"strings"
)

// ErrTargetMismatch は、繋ごうとしている相手と、その経路の接続先が食い違う
// ことを表す。
//
// コンテナはプロファイルの接続先ひとつだけを通す。食い違ったまま繋ぐと、
// 利用者が設定に書いた相手ではなく、プロファイルに書いた相手へ届く。どちらが
// 正しいかを推測せず、断る。
var ErrTargetMismatch = errors.New("the connection and its vpn profile name different targets")

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
//
// backend ごとの節は、Backend に合うものひとつだけを持つ。
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

// sameRouteAs は、この設定がもう一方と同じ経路を作るかを返す。
//
// 設定が変わったコンテナは作り直す。古い設定のまま繋ぎ続けると、利用者が直した
// 先へ行かない。
func (profile Profile) sameRouteAs(other Profile) bool {
	return reflect.DeepEqual(profile.normalized(), other.normalized())
}

// normalized は、意味の同じ書き方をひとつに揃える。DNS の nil と空の並びは同じ
// 「DNS を使わない」である。
func (profile Profile) normalized() Profile {
	if len(profile.DNS) == 0 {
		profile.DNS = nil
	}
	return profile
}

// Reaches は、この経路が address（`host:port`）へ繋ぐものかを返す。
func (profile Profile) Reaches(address string) bool { return profile.Target.Reaches(address) }

// Reaches は、address（`host:port`）がこの接続先を指すかを返す。
//
// 名前は大文字と小文字を区別せず、末尾の `.` を無視して比べる。DNS の名前として
// 同じものを、書き方の違いだけで食い違いとして断らない。
func (endpoint Endpoint) Reaches(address string) bool {
	host, port, err := net.SplitHostPort(address)
	if err != nil || port != fmt.Sprint(endpoint.Port) {
		return false
	}
	return sameHost(host, endpoint.Host)
}

func sameHost(left, right string) bool {
	return strings.EqualFold(strings.TrimSuffix(left, "."), strings.TrimSuffix(right, "."))
}

// Validate は、このプロファイルで経路を作れるかを確かめる。
func (profile Profile) Validate() error {
	if err := validateProfileName(profile.Name); err != nil {
		return err
	}
	chosen, err := backendFor(profile.Backend)
	if err != nil {
		return err
	}
	if err := validateResolvers(profile.DNS); err != nil {
		return err
	}
	if err := validateTarget(profile.Target, profile.DNS); err != nil {
		return err
	}
	if field := profile.foreignSection(); field != "" {
		return fieldError(ErrSettings, field, ReasonUnexpected)
	}
	return chosen.validateSettings(profile)
}

// foreignSection は、Backend と違う backend の節があれば、その名前を返す。
//
// 別の節が残っていると、作り直すかどうかの判断と、画面に出す設定が、使っていない
// 値に引きずられる。
func (profile Profile) foreignSection() string {
	sections := []struct {
		backend BackendName
		field   string
		present bool
	}{
		{WireGuard, "wireguard", profile.WireGuard != nil},
		{L2TPIPsec, "l2tp", profile.L2TP != nil},
		{OpenConnect, "openconnect", profile.OpenConnect != nil},
	}
	for _, section := range sections {
		if section.present && section.backend != profile.Backend {
			return section.field
		}
	}
	return ""
}

// OwnSecrets は、secrets のうち、このプロファイルの backend の節だけを残した写しを
// 返す。方式を切り替えたあとに、使わなくなった方式の秘密を Vault に残さない。
// この版の知らない backend なら、何も残さない。
func (profile Profile) OwnSecrets(secrets Secrets) Secrets {
	chosen, err := backendFor(profile.Backend)
	if err != nil {
		return Secrets{}
	}
	return chosen.ownSecrets(secrets)
}

// ValidateSecrets は、このbackendが要る秘密が揃っているかを確かめる。
func (profile Profile) ValidateSecrets(secrets Secrets) error {
	chosen, err := backendFor(profile.Backend)
	if err != nil {
		return err
	}
	return chosen.validateSecrets(profile, secrets)
}

// WaitsForApproval は、この経路が人の承認を待つかを返す。
func (profile Profile) WaitsForApproval() bool {
	chosen, err := backendFor(profile.Backend)
	return err == nil && chosen.waitsForApproval(profile)
}
