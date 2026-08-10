package application

import (
	"slices"

	"sshc/internal/config"
	"sshc/internal/keys"
)

// KeyHosts returns the concrete configured aliases whose effective
// IdentityFile values name each requested workspace-relative key path.
//
// The projection deliberately uses the same conservative expansion rules as
// the key inventory. Relative paths and unsupported tokens are not guesses,
// and open-ended Host patterns are expanded only through concrete aliases the
// application already knows how to show.
func (s *Service) KeyHosts(relativePaths []string) (map[string][]string, error) {
	hostsByKey := make(map[string][]string, len(relativePaths))
	absoluteByKey := make(map[string]string, len(relativePaths))
	for _, relative := range relativePaths {
		hostsByKey[relative] = []string{}
		absolute, err := AbsolutePath(s.workspace.Root(), relative)
		if err != nil {
			return nil, err
		}
		absoluteByKey[relative] = absolute
	}

	graph, err := s.resolve()
	if err != nil {
		return nil, err
	}
	projected, _ := ProjectHosts(graph, s.workspace.Root())
	seen := make(map[string]map[string]bool, len(absoluteByKey))
	for relative := range absoluteByKey {
		seen[relative] = map[string]bool{}
	}

	for _, host := range projected {
		alias := host.Identity.Alias
		if alias == "" || host.Duplicate {
			continue
		}
		for _, entry := range ComputeEffective(graph, s.workspace.Root(), alias).Entries {
			if !config.EqualKeyword(entry.Keyword, "IdentityFile") {
				continue
			}
			for _, value := range entry.Values {
				for relative, absolute := range absoluteByKey {
					if keys.ExpandsTo(s.workspace, value, absolute) {
						seen[relative][alias] = true
					}
				}
			}
		}
	}

	for relative, aliases := range seen {
		for alias := range aliases {
			hostsByKey[relative] = append(hostsByKey[relative], alias)
		}
		slices.Sort(hostsByKey[relative])
	}
	return hostsByKey, nil
}
