package application

import (
	"sshc/internal/config"
	"sshc/internal/remoteos"
	"sshc/internal/sshclient"
	"sshc/internal/storage"
)

// ObserveConnectionOS binds the eventual detection to the configuration that
// opened the connection. A deleted, moved or retargeted host is never updated.
func (s *Service) ObserveConnectionOS(target sshclient.Target) func(string) {
	graph, err := s.resolve()
	if err != nil {
		return nil
	}
	identity := s.connectionIdentity(graph, target.Alias)
	if identity.IsZero() {
		return nil
	}
	stored, _, err := s.metadata.Load()
	if err != nil {
		return nil
	}
	if index := hostMetadataIndex(stored.Hosts, identity); index >= 0 && stored.Hosts[index].OS != "" {
		return nil
	}
	binding := target.AuthenticationBinding()
	return func(name string) { _ = s.recordConnectionOS(identity, binding, name) }
}

func (s *Service) recordConnectionOS(identity HostIdentity, binding, name string) error {
	if name == "" || !remoteos.Valid(name) {
		return nil
	}
	s.saveMutex.Lock()
	defer s.saveMutex.Unlock()
	graph, err := s.resolve()
	if err != nil {
		return err
	}
	if !s.detectionApplies(graph, identity, binding) {
		return nil
	}
	stored, precondition, err := s.metadata.Load()
	if err != nil {
		return err
	}
	index := hostMetadataIndex(stored.Hosts, identity)
	if index < 0 {
		stored.Hosts = append(stored.Hosts, HostMetadata{Identity: identity})
		index = len(stored.Hosts) - 1
	}
	if host := stored.Hosts[index]; host.OS != "" || (host.DetectedOS == name && host.DetectedOSBinding == binding) {
		return nil
	}
	stored.Hosts[index].DetectedOS = name
	stored.Hosts[index].DetectedOSBinding = binding
	if err := s.metadata.EnsureDirectory(); err != nil {
		return err
	}
	change, err := s.metadata.Change(stored, precondition)
	if err != nil {
		return err
	}
	_, err = s.manager.Commit(storage.Request{Operation: "host.detect-os", Changes: []storage.Change{change}})
	return err
}

// detectionApplies は、認証の組み合わせ binding の接続で検出した OS が、いまの設定の
// identity のブロックにまだ当てはまるかを返す。ブロックが消えた、前のブロックに alias を
// すべて取られた、接続先や認証が変わった、のどれかなら当てはまらない。
//
// 確かめ直すのは、そのブロックへ接続する alias である。primary alias は前のブロックに
// 取られていることがあり（`Host a web` の後ろの `Host web b`）、それで確かめると別の
// ブロックの接続を見てしまう。
func (s *Service) detectionApplies(graph *config.Graph, identity HostIdentity, binding string) bool {
	alias, found := s.connectingAlias(graph, identity)
	if !found {
		return false
	}
	current, err := s.passwordBindingForGraph(graph, alias)
	return err == nil && current == binding
}
