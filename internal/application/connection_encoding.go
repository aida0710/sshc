package application

import (
	"strings"

	"sshc/internal/textencoding"
)

// ConnectionEncoding returns the text encoding attached to the concrete Host
// block that wins for alias. The default is UTF-8 and is intentionally not
// stored, so future defaults remain an application decision.
func (s *Service) ConnectionEncoding(alias string) (textencoding.Name, error) {
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
	// External and wildcard-only rules have no editable identity, so they cannot
	// own sshc metadata. Do not borrow a same-named internal host's setting.
	if identity.IsZero() {
		return textencoding.UTF8, nil
	}

	stored, _, err := s.metadata.Load()
	if err != nil {
		return "", err
	}
	for _, host := range stored.Hosts {
		if host.Identity == identity {
			return textencoding.Parse(host.Encoding)
		}
	}
	return textencoding.UTF8, nil
}
