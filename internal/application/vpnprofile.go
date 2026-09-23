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
	// WireGuard は、backend が wireguard のときの設定である。
	WireGuard *WireGuardProfile `json:"wireguard,omitempty"`
	// L2TP は、backend が l2tp_ipsec のときの設定である。
	L2TP *L2TPProfile `json:"l2tp,omitempty"`
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
func (stored VPNProfile) Profile() (vpn.Profile, error) {
	target, err := parseEndpoint(stored.Target)
	if err != nil {
		return vpn.Profile{}, fmt.Errorf("%w: 接続先 %q: %w", ErrMetadataVPN, stored.Target, err)
	}
	profile := vpn.Profile{
		Name:    stored.Name,
		Backend: vpn.BackendName(stored.Backend),
		Target:  target,
	}
	if stored.WireGuard != nil {
		server, err := parseEndpoint(stored.WireGuard.Server)
		if err != nil {
			return vpn.Profile{}, fmt.Errorf("%w: サーバー %q: %w", ErrMetadataVPN, stored.WireGuard.Server, err)
		}
		profile.WireGuard = &vpn.WireGuardSettings{
			Server:        server,
			PeerPublicKey: stored.WireGuard.PeerPublicKey,
			Address:       stored.WireGuard.Address,
		}
	}
	if stored.L2TP != nil {
		profile.L2TP = &vpn.L2TPSettings{
			Server:   stored.L2TP.Server,
			Username: stored.L2TP.Username,
			IKE:      stored.L2TP.IKE,
			ESP:      stored.L2TP.ESP,
		}
	}
	if err := profile.Validate(); err != nil {
		return vpn.Profile{}, fmt.Errorf("%w: %w", ErrMetadataVPN, err)
	}
	return profile, nil
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
		if backend := vpn.BackendName(stored.Backend); backend != vpn.WireGuard && backend != vpn.L2TPIPsec {
			continue
		}
		if _, err := stored.Profile(); err != nil {
			return err
		}
	}
	return nil
}
