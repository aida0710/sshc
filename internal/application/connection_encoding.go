package application

import "sshc/internal/textencoding"

// ConnectionEncoding returns the text encoding attached to the concrete Host
// block that wins for alias. The default is UTF-8 and is intentionally not
// stored, so future defaults remain an application decision.
func (s *Service) ConnectionEncoding(alias string) (textencoding.Name, error) {
	graph, err := s.resolve()
	if err != nil {
		return "", err
	}
	identity := s.connectionIdentity(graph, alias)
	// External and wildcard-only rules have no editable identity, so they cannot
	// own sshc metadata. Do not borrow a same-named internal host's setting.
	if identity.IsZero() {
		return textencoding.UTF8, nil
	}

	stored, _, err := s.metadata.Load()
	if err != nil {
		return "", err
	}
	if index := hostMetadataIndex(stored.Hosts, identity); index >= 0 {
		return textencoding.Parse(stored.Hosts[index].Encoding)
	}
	return textencoding.UTF8, nil
}
