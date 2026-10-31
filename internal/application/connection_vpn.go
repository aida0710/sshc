package application

import (
	"fmt"
	"sort"

	"sshc/internal/vpn"
)

// 接続とVPNプロファイルの紐付けを読む。書き込みは metadata の保存と同じ経路を通る。

// AttachedVPNProfile は、接続に付けたVPNプロファイルである。Name は経路の起動に、ID は
// 保存済みのパスワードなどの結び付けの値（sshclient.Target.VPNProfileID）に使う。
//
// Name が空なら、この接続はVPNを通らない。付けた名前のプロファイルが無ければ ID が空に
// なる。その接続は経路を起動できない。
type AttachedVPNProfile struct {
	Name string
	ID   string
}

// ConnectionVPN は、この alias へ届くために通るVPNプロファイルの名前を返す。
//
// 空なら、この接続はVPNを通らない。
func (s *Service) ConnectionVPN(alias string) (string, error) {
	attached, err := s.ConnectionVPNProfile(alias)
	return attached.Name, err
}

// ConnectionVPNProfile は、この alias へ届くために通るVPNプロファイルの名前と識別子を返す。
func (s *Service) ConnectionVPNProfile(alias string) (AttachedVPNProfile, error) {
	graph, err := s.resolve()
	if err != nil {
		return AttachedVPNProfile{}, err
	}
	identity := s.connectionIdentity(graph, alias)
	// 外部ファイルやワイルドカードだけの規則は編集できる identity を持たない。
	// 同じ名前の内部ホストの設定を借りない。
	if identity.IsZero() {
		return AttachedVPNProfile{}, nil
	}
	stored, _, err := s.metadata.Load()
	if err != nil {
		return AttachedVPNProfile{}, err
	}
	return attachedVPNProfile(stored, identity), nil
}

// attachedVPNProfile は、identity のブロックに付けたVPNプロファイルを metadata から返す。
// 付けていなければ空である。
func attachedVPNProfile(stored Metadata, identity HostIdentity) AttachedVPNProfile {
	if identity.IsZero() {
		return AttachedVPNProfile{}
	}
	index := hostMetadataIndex(stored.Hosts, identity)
	if index < 0 || stored.Hosts[index].VPN == "" {
		return AttachedVPNProfile{}
	}
	attached := AttachedVPNProfile{Name: stored.Hosts[index].VPN}
	if profile := vpnProfileIndex(stored.VPNProfiles, attached.Name); profile >= 0 {
		attached.ID = stored.VPNProfiles[profile].ID
	}
	return attached
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

// StoredVPNProfile は、名前で保存済みのVPNプロファイルを、metadata.json に書いた形で返す。
func (s *Service) StoredVPNProfile(name string) (VPNProfile, error) {
	stored, _, err := s.metadata.Load()
	if err != nil {
		return VPNProfile{}, err
	}
	for _, profile := range stored.VPNProfiles {
		if profile.Name == name {
			return profile, nil
		}
	}
	return VPNProfile{}, fmt.Errorf("%w: %s", ErrUnknownVPNProfile, name)
}

// VPNProfiles は、保存済みのプロファイルを名前順で返す。秘密は含まない。
func (s *Service) VPNProfiles() ([]VPNProfile, error) {
	stored, _, err := s.metadata.Load()
	if err != nil {
		return nil, err
	}
	profiles := append([]VPNProfile(nil), stored.VPNProfiles...)
	sort.Slice(profiles, func(left, right int) bool { return profiles[left].Name < profiles[right].Name })
	return profiles, nil
}

// SetConnectionVPN は、接続が通るプロファイルを決める。空なら紐付けを外す。
func (s *Service) SetConnectionVPN(alias, profile string) (SaveResult, error) {
	stored, precondition, err := s.metadata.Load()
	if err != nil {
		return SaveResult{}, err
	}
	if profile != "" && vpnProfileIndex(stored.VPNProfiles, profile) < 0 {
		return SaveResult{}, fmt.Errorf("%w: %s", ErrUnknownVPNProfile, profile)
	}
	identity, err := s.hostIdentity(alias)
	if err != nil {
		return SaveResult{}, err
	}
	if index := hostMetadataIndex(stored.Hosts, identity); index >= 0 {
		stored.Hosts[index].VPN = profile
	} else {
		stored.Hosts = append(stored.Hosts, HostMetadata{Identity: identity, VPN: profile})
	}
	return s.commitMetadata(stored, precondition, "vpn.connection.bind")
}

// hostIdentity は、alias が指す編集できるホストを返す。
func (s *Service) hostIdentity(alias string) (HostIdentity, error) {
	graph, err := s.resolve()
	if err != nil {
		return HostIdentity{}, err
	}
	identity := s.connectionIdentity(graph, alias)
	if identity.IsZero() {
		return HostIdentity{}, fmt.Errorf("%w: %s", ErrUnknownConnection, alias)
	}
	return identity, nil
}

// VPNBindings は、プロファイル名ごとに、それを通る接続の alias を返す。
func (s *Service) VPNBindings() (map[string][]string, error) {
	stored, _, err := s.metadata.Load()
	if err != nil {
		return nil, err
	}
	bindings := map[string][]string{}
	for _, host := range stored.Hosts {
		if host.VPN == "" {
			continue
		}
		bindings[host.VPN] = append(bindings[host.VPN], host.Identity.Alias)
	}
	for _, aliases := range bindings {
		sort.Strings(aliases)
	}
	return bindings, nil
}
