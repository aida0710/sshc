package application

import (
	"strings"

	"sshc/internal/config"
	"sshc/internal/remoteos"
	"sshc/internal/sshclient"
	"sshc/internal/storage"
)

func (s *Service) osIdentity(graph *config.Graph, alias string) HostIdentity {
	hosts, _ := ProjectHosts(graph, s.workspace.Root())
	for _, host := range hosts {
		if strings.EqualFold(host.Identity.Alias, alias) {
			return host.Identity
		}
	}
	return HostIdentity{}
}

// ObserveConnectionOS binds the eventual detection to the configuration that
// opened the connection. A deleted, moved or retargeted host is never updated.
func (s *Service) ObserveConnectionOS(target sshclient.Target) func(string) {
	graph, err := s.resolve()
	if err != nil {
		return nil
	}
	identity := s.osIdentity(graph, target.Alias)
	if identity.IsZero() {
		return nil
	}
	stored, _, err := s.metadata.Load()
	if err != nil {
		return nil
	}
	for _, host := range stored.Hosts {
		if host.Identity == identity && host.OS != "" {
			return nil
		}
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
	if s.osIdentity(graph, identity.Alias) != identity {
		return nil
	}
	current, err := s.passwordBindingForGraph(graph, identity.Alias)
	if err != nil {
		return err
	}
	if current != binding {
		return nil
	}
	stored, precondition, err := s.metadata.Load()
	if err != nil {
		return err
	}
	index := -1
	for i, host := range stored.Hosts {
		if host.Identity != identity {
			continue
		}
		if host.OS != "" || (host.DetectedOS == name && host.DetectedOSBinding == binding) {
			return nil
		}
		index = i
		break
	}
	if index < 0 {
		stored.Hosts = append(stored.Hosts, HostMetadata{Identity: identity})
		index = len(stored.Hosts) - 1
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
