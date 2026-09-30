package application

import (
	"errors"
	"fmt"

	"sshc/internal/vpn"
)

// VPN プロファイルの保存形式である。秘密は Vault にあり、この文書には無い。

var (
	// ErrMetadataVPN は、保存された VPN プロファイルが使えないことを表す。
	ErrMetadataVPN = errors.New("metadata vpn profile is invalid")
	// ErrUnknownVPNProfile は、その名前のプロファイルが無いことを表す。
	ErrUnknownVPNProfile = errors.New("that vpn profile is not configured")
	// ErrUnknownConnection は、その alias で編集できる接続が無いことを表す。
	// 外部ファイルやワイルドカードだけの規則は sshc の metadata を持てない。
	ErrUnknownConnection = errors.New("that connection cannot hold sshc metadata")
)

// VPNProfile は、metadata.json に保存する VPN 経路ひとつぶんである。
//
// 接続先は持たない。接続先は、このプロファイルを付けた接続（hosts[].vpn）の
// HostName と Port で決まる。
type VPNProfile struct {
	// ID は、sshc が作るときに決める識別子である（vpnprofile_id.go）。改名と編集では
	// 変えない。保存の要求に書かれた値は使わない。
	ID      string          `json:"id,omitempty"`
	Name    string          `json:"name"`
	Backend vpn.BackendName `json:"backend"`
	// DNS は、接続先を VPN の中で名前解決するための DNS サーバーである。
	// 接続先をアドレスで書くなら要らない。
	DNS []string `json:"dns,omitempty"`
	// WireGuard は、backend が wireguard のときの設定である。
	WireGuard *WireGuardProfile `json:"wireguard,omitempty"`
	// L2TP は、backend が l2tp_ipsec のときの設定である。
	L2TP *L2TPProfile `json:"l2tp,omitempty"`
	// OpenConnect は、backend が openconnect のときの設定である。
	OpenConnect *OpenConnectProfile `json:"openconnect,omitempty"`
	// OpenVPN は、backend が openvpn のときの設定である。
	OpenVPN *OpenVPNProfile `json:"openvpn,omitempty"`
	// IKEv2 は、backend が ikev2 のときの設定である。
	IKEv2 *IKEv2Profile `json:"ikev2,omitempty"`
}

// OpenConnectProfile は、openconnect backend の秘密でない設定である。
type OpenConnectProfile struct {
	// Server は、VPN装置の名前またはアドレスである。名前はコンテナの中で引く。
	Server string `json:"server"`
	// Username は、VPNの利用者名である。パスワードは Vault にある。
	Username string `json:"username"`
	// Protocol は、その装置が話す方式である。空なら anyconnect。
	Protocol string `json:"protocol,omitempty"`
	// ServerCertificate は、サーバーの証明書を固定するフィンガープリントである
	// （`sha256:...` または `pin-sha256:...`）。
	ServerCertificate string `json:"serverCertificate,omitempty"`
	// SecondFactor は、二段目の質問への答え方である（`approve` または `totp`）。
	// 空なら答えない。
	SecondFactor string `json:"secondFactor,omitempty"`
	// ApprovalWord は、SecondFactor が approve のときに、サーバーが追加の入力を
	// 求めたら送る文字列である（Duo なら push）。空なら何も送らない。
	ApprovalWord string `json:"approvalWord,omitempty"`
}

// L2TPProfile は、l2tp_ipsec backend の秘密でない設定である。
type L2TPProfile struct {
	// Server は、VPN装置の名前またはアドレスである。名前はコンテナの中で引く。
	Server string `json:"server"`
	// Username は、VPNの利用者名である。パスワードは Vault にある。
	Username string `json:"username"`
	// IKE と ESP は、古い装置と暗号方式が合わないときだけ書く。
	IKE string `json:"ike,omitempty"`
	ESP string `json:"esp,omitempty"`
}

// WireGuardProfile は、wireguard backend の秘密でない設定である。設定ファイルは鍵を含むので
// Vault にある。
type WireGuardProfile struct {
	// Servers は、設定ファイルの Endpoint のサーバーである。一覧に出す。
	Servers []string `json:"servers"`
	// Server、PeerPublicKey、Address は、v0.40.0 までの項目の形で保存したプロファイルだけが
	// 持つ（秘密鍵は Vault の wireguardPrivateKey）。経路を起動するときと、編集で開くときは、
	// これと秘密鍵から設定ファイルを組み立てる（internal/vpnprofile）。保存し直すと、設定
	// ファイルの形になって消える。
	Server        string `json:"server,omitempty"`
	PeerPublicKey string `json:"peerPublicKey,omitempty"`
	Address       string `json:"address,omitempty"`
}

// WireGuardFields は、v0.40.0 までの項目の形のプロファイルなら、その項目を返す。
func (stored VPNProfile) WireGuardFields() (vpn.WireGuardFields, bool) {
	settings := stored.WireGuard
	if stored.Backend != vpn.WireGuard || settings == nil || settings.Server == "" {
		return vpn.WireGuardFields{}, false
	}
	return vpn.WireGuardFields{
		Server: settings.Server, PeerPublicKey: settings.PeerPublicKey, Address: settings.Address, DNS: stored.DNS,
	}, true
}

// withoutWireGuardFields は、v0.40.0 までの項目を落とした写しを返す。保存し直すプロファイルの設定
// ファイルは、Vault に書く。
func (stored VPNProfile) withoutWireGuardFields() VPNProfile {
	if stored.WireGuard == nil {
		return stored
	}
	stored.WireGuard = &WireGuardProfile{Servers: stored.WireGuard.Servers}
	return stored
}

// Profile は、保存した設定を internal/vpn が使う形へ直す。
//
// どの節もそのまま移す。backend と違う節があれば、vpn.Profile.Validate がその節の名前を
// 項目にして断る。
func (stored VPNProfile) Profile() (vpn.Profile, error) {
	profile := vpn.Profile{
		Name:    stored.Name,
		Backend: stored.Backend,
		DNS:     append([]string(nil), stored.DNS...),
	}
	if settings := stored.WireGuard; settings != nil {
		profile.WireGuard = &vpn.WireGuardSettings{Servers: append([]string(nil), settings.Servers...)}
	}
	if fields, found := stored.WireGuardFields(); found {
		if err := fields.Validate(); err != nil {
			return vpn.Profile{}, fmt.Errorf("%w: %w", ErrMetadataVPN, err)
		}
	}
	if settings := stored.OpenConnect; settings != nil {
		profile.OpenConnect = &vpn.OpenConnectSettings{
			Server:            settings.Server,
			Username:          settings.Username,
			Protocol:          settings.Protocol,
			ServerCertificate: settings.ServerCertificate,
			SecondFactor:      settings.SecondFactor,
			ApprovalWord:      settings.ApprovalWord,
		}
	}
	if settings := stored.L2TP; settings != nil {
		profile.L2TP = &vpn.L2TPSettings{
			Server: settings.Server, Username: settings.Username, IKE: settings.IKE, ESP: settings.ESP,
		}
	}
	if settings := stored.OpenVPN; settings != nil {
		profile.OpenVPN = settings.openVPNSettings()
	}
	if settings := stored.IKEv2; settings != nil {
		profile.IKEv2 = settings.ikev2Settings()
	}
	if err := profile.Validate(); err != nil {
		return vpn.Profile{}, fmt.Errorf("%w: %w", ErrMetadataVPN, err)
	}
	return profile, nil
}

// validateVPNProfiles は、保存してよい形かを確かめる。
//
// 形の変わった保存形式は、metadata の schemaVersion を上げて移行する。古いバージョンは
// 新しい schemaVersion の文書を読まないので、ここは知っている形だけを通す。
func validateVPNProfiles(profiles []VPNProfile) error {
	seen := map[string]bool{}
	seenIDs := map[string]bool{}
	for _, stored := range profiles {
		if err := vpn.ValidateName(stored.Name); err != nil {
			return fmt.Errorf("%w: %w", ErrMetadataVPN, err)
		}
		if seen[stored.Name] {
			return fmt.Errorf("%w: 同じ名前が二つあります: %s", ErrMetadataVPN, stored.Name)
		}
		seen[stored.Name] = true
		// 識別子は割り当ての結び付けの値に入る。無い、または2つのプロファイルで同じだと、
		// 別のプロファイルを通る接続に割り当てが渡りうる。
		if !validVPNProfileID(stored.ID) {
			return fmt.Errorf("%w: %s has no valid id", ErrMetadataVPN, stored.Name)
		}
		if seenIDs[stored.ID] {
			return fmt.Errorf("%w: %s shares its id with another profile", ErrMetadataVPN, stored.Name)
		}
		seenIDs[stored.ID] = true
		if _, err := stored.Profile(); err != nil {
			return err
		}
	}
	return nil
}
