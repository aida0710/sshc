package application

import (
	"fmt"
	"sort"
	"strings"

	"sshc/internal/storage"
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

// SaveVPNProfile は、プロファイルひとつを保存する。同じ名前があれば置き換える。
//
// 秘密はここを通らない。Vault が持つ。
func (s *Service) SaveVPNProfile(profile VPNProfile) (SaveResult, error) {
	if _, err := profile.Profile(); err != nil {
		return SaveResult{}, err
	}
	stored, precondition, err := s.metadata.Load()
	if err != nil {
		return SaveResult{}, err
	}
	replaced := false
	for index, existing := range stored.VPNProfiles {
		if existing.Name == profile.Name {
			stored.VPNProfiles[index] = profile
			replaced = true
			break
		}
	}
	if !replaced {
		stored.VPNProfiles = append(stored.VPNProfiles, profile)
	}
	return s.commitMetadata(stored, precondition, "vpn.profile.save")
}

// RemoveVPNProfile は、プロファイルと、それを指している接続の紐付けを同時に消す。
//
// 別々に消すと、消えたプロファイルを指したままの接続が残る。その接続は繋ぐ
// たびに「そのプロファイルは無い」と断られ、利用者は設定のどこを直せばよいかを
// 探すことになる。
func (s *Service) RemoveVPNProfile(name string) (SaveResult, error) {
	stored, precondition, err := s.metadata.Load()
	if err != nil {
		return SaveResult{}, err
	}
	profiles := make([]VPNProfile, 0, len(stored.VPNProfiles))
	found := false
	for _, existing := range stored.VPNProfiles {
		if existing.Name == name {
			found = true
			continue
		}
		profiles = append(profiles, existing)
	}
	if !found {
		return SaveResult{}, fmt.Errorf("%w: %s", ErrUnknownVPNProfile, name)
	}
	stored.VPNProfiles = profiles
	for index, host := range stored.Hosts {
		if host.VPN == name {
			stored.Hosts[index].VPN = ""
		}
	}
	return s.commitMetadata(stored, precondition, "vpn.profile.remove")
}

// SetConnectionVPN は、接続が通るプロファイルを決める。空なら紐付けを外す。
func (s *Service) SetConnectionVPN(alias, profile string) (SaveResult, error) {
	stored, precondition, err := s.metadata.Load()
	if err != nil {
		return SaveResult{}, err
	}
	if profile != "" {
		known := false
		for _, existing := range stored.VPNProfiles {
			if existing.Name == profile {
				known = true
				break
			}
		}
		if !known {
			return SaveResult{}, fmt.Errorf("%w: %s", ErrUnknownVPNProfile, profile)
		}
	}
	identity, err := s.hostIdentity(alias)
	if err != nil {
		return SaveResult{}, err
	}
	updated := false
	for index, host := range stored.Hosts {
		if host.Identity == identity {
			stored.Hosts[index].VPN = profile
			updated = true
			break
		}
	}
	if !updated {
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
	hosts, _ := ProjectHosts(graph, s.workspace.Root())
	for _, host := range hosts {
		if strings.EqualFold(host.Identity.Alias, alias) {
			return host.Identity, nil
		}
	}
	return HostIdentity{}, fmt.Errorf("%w: %s", ErrUnknownConnection, alias)
}

func (s *Service) commitMetadata(stored Metadata, precondition storage.Precondition, operation string) (SaveResult, error) {
	if err := s.metadata.EnsureDirectory(); err != nil {
		return SaveResult{}, err
	}
	change, err := s.metadata.Change(stored, precondition)
	if err != nil {
		return SaveResult{}, err
	}
	result, err := s.manager.Commit(storage.Request{Operation: operation, Changes: []storage.Change{change}})
	if err != nil {
		return SaveResult{}, err
	}
	return SaveResult{TransactionID: result.ID, Written: result.Written}, nil
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
