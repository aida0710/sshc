package httpserver

import "sshc/internal/application"

// validateHostEdits は、一括操作の対象を確かめる。各接続先の path は bases のどれか 1 つと
// 同じ文字列で、bases の各ファイルはどれかの接続先に使われていなければならない。
func validateHostEdits(hosts []application.HostEdit, bases []application.FileBase) error {
	if len(hosts) == 0 || len(hosts) > application.MaxHostEdits || len(bases) == 0 || len(bases) > application.MaxHostEdits {
		return errInvalidEdit
	}
	unused := map[string]bool{}
	for _, base := range bases {
		if err := validatePathParameter(base.Path); err != nil {
			return err
		}
		if len(base.Base) > maxRawLength || unused[base.Path] {
			return errInvalidEdit
		}
		unused[base.Path] = true
	}
	seen := map[application.HostIdentity]bool{}
	for _, host := range hosts {
		if err := validateHostBlockAlias(host.Alias); err != nil {
			return err
		}
		identity := application.HostIdentity{Path: host.Path, Alias: host.Alias}
		if _, declared := unused[host.Path]; !declared || seen[identity] {
			return errInvalidEdit
		}
		seen[identity] = true
		unused[host.Path] = false
	}
	for _, left := range unused {
		if left {
			return errInvalidEdit
		}
	}
	return nil
}
