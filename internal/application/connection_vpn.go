package application

import (
	"fmt"
	"strings"

	"sshc/internal/vpn"
)

// 接続とVPNプロファイルの紐付けを読む。書き込みは metadata の保存と同じ経路を通る。

// ConnectionVPN は、この alias へ届くために通るVPNプロファイルの名前を返す。
//
// 空なら、この接続はVPNを通らない。
func (s *Service) ConnectionVPN(alias string) (string, error) {
	graph, err := s.resolve()
	if err != nil {
		return "", err
	}
	hosts, _ := ProjectHosts(graph, s.workspace.Root())
	var identity HostIdentity
	for _, host := range hosts {
		if strings.EqualFold(host.Identity.Alias, alias) {
			identity = host.Identity
			break
		}
	}
	// 外部ファイルやワイルドカードだけの規則は編集できる identity を持たない。
	// 同じ名前の内部ホストの設定を借りない。
	if identity.IsZero() {
		return "", nil
	}
	stored, _, err := s.metadata.Load()
	if err != nil {
		return "", err
	}
	for _, host := range stored.Hosts {
		if host.Identity == identity {
			return host.VPN, nil
		}
	}
	return "", nil
}

// VPNProfile は、名前で保存済みのVPNプロファイルを返す。
//
// 無ければ理由を返す。接続の直前に「そのプロファイルは無い」と分かる方が、
// 経路を作れないまま素の回線で繋ぐより良い。
func (s *Service) VPNProfile(name string) (vpn.Profile, error) {
	stored, _, err := s.metadata.Load()
	if err != nil {
		return vpn.Profile{}, err
	}
	for _, profile := range stored.VPNProfiles {
		if profile.Name == name {
			return profile.Profile()
		}
	}
	return vpn.Profile{}, fmt.Errorf("%w: %s", ErrUnknownVPNProfile, name)
}

// VPNProfiles は、保存済みのプロファイルを名前順で返す。秘密は含まない。
func (s *Service) VPNProfiles() ([]VPNProfile, error) {
	stored, _, err := s.metadata.Load()
	if err != nil {
		return nil, err
	}
	profiles := append([]VPNProfile(nil), stored.VPNProfiles...)
	return profiles, nil
}
