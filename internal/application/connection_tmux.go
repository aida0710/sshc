package application

// ConnectionTmux returns whether the concrete Host block that wins for alias
// opens in tmux. External and wildcard-only rules have no editable identity, so
// they cannot own this setting, as with ConnectionEncoding.
func (s *Service) ConnectionTmux(alias string) (bool, error) {
	graph, err := s.resolve()
	if err != nil {
		return false, err
	}
	identity := s.connectionIdentity(graph, alias)
	if identity.IsZero() {
		return false, nil
	}
	stored, _, err := s.metadata.Load()
	if err != nil {
		return false, err
	}
	if index := hostMetadataIndex(stored.Hosts, identity); index >= 0 {
		return stored.Hosts[index].Tmux, nil
	}
	return false, nil
}
