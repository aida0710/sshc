package application

import (
	"errors"
	"fmt"
	"net"
	"strconv"

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
// 接続先はひとつに限る。VPN の向こうのネットワーク全体を引き込まないので、
// 経路とパケットフィルタが接続先ひとつで閉じる。
type VPNProfile struct {
	Name    string `json:"name"`
	Backend string `json:"backend"`
	// Target は、この VPN の中にある接続先である。`host:port` で書く。
	Target string `json:"target"`
	// DNS は、接続先の名前を VPN の中で引くための DNS サーバーである。
	// 接続先をアドレスで書くなら要らない。
	DNS []string `json:"dns,omitempty"`
	// WireGuard は、backend が wireguard のときの設定である。
	WireGuard *WireGuardProfile `json:"wireguard,omitempty"`
	// L2TP は、backend が l2tp_ipsec のときの設定である。
	L2TP *L2TPProfile `json:"l2tp,omitempty"`
	// OpenConnect は、backend が openconnect のときの設定である。
	OpenConnect *OpenConnectProfile `json:"openconnect,omitempty"`
}

// OpenConnectProfile は、openconnect backend の秘密でない設定である。
type OpenConnectProfile struct {
	// Server は、VPN装置の名前またはアドレスである。名前はコンテナの中で引く。
	Server string `json:"server"`
	// Username は、VPNの利用者名である。パスワードは Vault にある。
	Username string `json:"username"`
	// Protocol は、その装置が話す方式である。空なら anyconnect。
	Protocol string `json:"protocol,omitempty"`
	// ServerCertificate は、相手の証明書を固定する指紋である（`sha256:...`）。
	ServerCertificate string `json:"serverCertificate,omitempty"`
	// SecondFactor は、二段目の質問への答え方である（`approve` または `totp`）。
	// 空なら答えない。
	SecondFactor string `json:"secondFactor,omitempty"`
	// ApprovalWord は、SecondFactor が approve のときに送る語である。空なら push。
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

// WireGuardProfile は、wireguard backend の秘密でない設定である。
type WireGuardProfile struct {
	// Server は、トンネルの相手である。`host:port` で書く。
	Server string `json:"server"`
	// PeerPublicKey は、相手の公開鍵である。秘密ではない。
	PeerPublicKey string `json:"peerPublicKey"`
	// Address は、トンネル側でこの端末が名乗るアドレスである（CIDR 表記）。
	Address string `json:"address"`
}

// Profile は、保存した設定を internal/vpn が使う形へ直す。
//
// backend に合う節だけを移す。古い版や別の画面が残した、使っていない節には
// 引きずられない。
func (stored VPNProfile) Profile() (vpn.Profile, error) {
	target, err := parseEndpoint(stored.Target)
	if err != nil {
		return vpn.Profile{}, metadataVPNFieldError(vpn.ErrTarget, "target")
	}
	normalized := stored.Normalized()
	profile := vpn.Profile{
		Name:    normalized.Name,
		Backend: vpn.BackendName(normalized.Backend),
		Target:  target,
		DNS:     append([]string(nil), normalized.DNS...),
	}
	if settings := normalized.WireGuard; settings != nil {
		server, err := parseEndpoint(settings.Server)
		if err != nil {
			return vpn.Profile{}, metadataVPNFieldError(vpn.ErrSettings, "wireguard.server")
		}
		profile.WireGuard = &vpn.WireGuardSettings{
			Server: server, PeerPublicKey: settings.PeerPublicKey, Address: settings.Address,
		}
	}
	if settings := normalized.OpenConnect; settings != nil {
		profile.OpenConnect = &vpn.OpenConnectSettings{
			Server:            settings.Server,
			Username:          settings.Username,
			Protocol:          settings.Protocol,
			ServerCertificate: settings.ServerCertificate,
			SecondFactor:      settings.SecondFactor,
			ApprovalWord:      settings.ApprovalWord,
		}
	}
	if settings := normalized.L2TP; settings != nil {
		profile.L2TP = &vpn.L2TPSettings{
			Server: settings.Server, Username: settings.Username, IKE: settings.IKE, ESP: settings.ESP,
		}
	}
	if err := profile.Validate(); err != nil {
		return vpn.Profile{}, fmt.Errorf("%w: %w", ErrMetadataVPN, err)
	}
	return profile, nil
}

// Reaches は、この経路の接続先が address（`host:port`）を指すかを返す。
// 比べ方は vpn.Endpoint.Reaches と同じである。
func (stored VPNProfile) Reaches(address string) bool {
	target, err := parseEndpoint(stored.Target)
	return err == nil && target.Reaches(address)
}

// Normalized は、backend と違う節を落とし、空の DNS を無しに揃えた写しを返す。
//
// 画面は方式を切り替えたあとに古い節を送ってくることがある。断らずに落とすのは、
// 利用者が直せる誤りではないからである。
func (stored VPNProfile) Normalized() VPNProfile {
	normalized := stored
	if len(normalized.DNS) == 0 {
		normalized.DNS = nil
	}
	if normalized.Backend != string(vpn.WireGuard) {
		normalized.WireGuard = nil
	}
	if normalized.Backend != string(vpn.L2TPIPsec) {
		normalized.L2TP = nil
	}
	if normalized.Backend != string(vpn.OpenConnect) {
		normalized.OpenConnect = nil
	}
	return normalized
}

// metadataVPNFieldError は、保存形式の `host:port` が読めないことを、項目の
// 誤りとして返す。
func metadataVPNFieldError(kind error, field string) error {
	return fmt.Errorf("%w: %w", ErrMetadataVPN, &vpn.FieldError{Kind: kind, Field: field, Reason: vpn.ReasonFormat})
}

func parseEndpoint(value string) (vpn.Endpoint, error) {
	host, port, err := net.SplitHostPort(value)
	if err != nil {
		return vpn.Endpoint{}, err
	}
	number, err := strconv.Atoi(port)
	if err != nil {
		return vpn.Endpoint{}, err
	}
	return vpn.Endpoint{Host: host, Port: number}, nil
}

// validateVPNProfiles は、保存してよい形かを確かめる。
//
// この版が知らない backend は、名前の形だけを見て通す。プロファイルは端末の
// あいだで同期されるので、新しい版が書いたものを古い版が読み書きすることが
// ある。知らないという理由で文書ごと拒むと、その端末では metadata を保存
// できなくなる。使えるかどうかは、繋ぐときに改めて確かめる。
func validateVPNProfiles(profiles []VPNProfile) error {
	seen := map[string]bool{}
	for _, stored := range profiles {
		if err := vpn.ValidateName(stored.Name); err != nil {
			return fmt.Errorf("%w: %w", ErrMetadataVPN, err)
		}
		if seen[stored.Name] {
			return fmt.Errorf("%w: 同じ名前が二つあります: %s", ErrMetadataVPN, stored.Name)
		}
		seen[stored.Name] = true
		if stored.Backend == "" {
			return fmt.Errorf("%w: %s に backend がありません", ErrMetadataVPN, stored.Name)
		}
		if !vpn.KnownBackend(stored.Backend) {
			continue
		}
		if _, err := stored.Profile(); err != nil {
			return err
		}
	}
	return nil
}
