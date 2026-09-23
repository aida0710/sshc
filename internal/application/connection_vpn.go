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

// commitMetadata は、metadata だけを書く。
func (s *Service) commitMetadata(stored Metadata, precondition storage.Precondition, operation string) (SaveResult, error) {
	return s.commitMetadataWith(metadataCommit{operation: operation, metadata: stored, precondition: precondition}, nil)
}

// metadataCommit は、書く前の metadata と、読んだときの前提である。
type metadataCommit struct {
	operation    string
	metadata     Metadata
	precondition storage.Precondition
}

// commitMetadataWith は、metadata と、あれば別の変更（vault など）を、ひとつの
// storage.Request で書く。どちらかだけが書かれることはない。
func (s *Service) commitMetadataWith(planned metadataCommit, alongside *storage.Change) (SaveResult, error) {
	if err := s.metadata.EnsureDirectory(); err != nil {
		return SaveResult{}, err
	}
	change, err := s.metadata.Change(planned.metadata, planned.precondition)
	if err != nil {
		return SaveResult{}, err
	}
	changes := []storage.Change{change}
	if alongside != nil {
		changes = append(changes, *alongside)
	}
	result, err := s.manager.Commit(storage.Request{Operation: planned.operation, Changes: changes})
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
