package httpserver

import "sshc/internal/application"

func validateHostEdits(hosts []application.HostEdit) error {
	if len(hosts) == 0 || len(hosts) > application.MaxHostEdits {
		return errInvalidEdit
	}
	bases := map[string]string{}
	seen := map[application.HostIdentity]bool{}
	for _, host := range hosts {
		if err := validatePathParameter(host.Path); err != nil {
			return err
		}
		if err := validateHostBlockAlias(host.Alias); err != nil {
			return err
		}
		if len(host.Base) > maxRawLength {
			return errInvalidEdit
		}
		if base, exists := bases[host.Path]; exists && base != host.Base {
			return errInvalidEdit
		}
		identity := application.HostIdentity{Path: host.Path, Alias: host.Alias}
		if seen[identity] {
			return errInvalidEdit
		}
		bases[host.Path], seen[identity] = host.Base, true
	}
	return nil
}
