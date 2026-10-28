package httpserver

import "sshc/internal/application"

// validateHostEdits は、一括操作の対象の形だけを確かめる（件数、パスと接続名の書き方、中身の
// 大きさ）。接続先とファイルの対応（重複、どのファイルも使われること）は、パスを解決して
// 比べる application の loadHostEditSources が確かめる。
func validateHostEdits(hosts []application.HostIdentity, bases []application.FileBase) error {
	if len(hosts) == 0 || len(hosts) > application.MaxHostEdits || len(bases) == 0 || len(bases) > application.MaxHostEdits {
		return errInvalidEdit
	}
	for _, base := range bases {
		if err := validatePathParameter(base.Path); err != nil {
			return err
		}
		if len(base.Base) > maxRawLength {
			return errInvalidEdit
		}
	}
	for _, host := range hosts {
		if err := validatePathParameter(host.Path); err != nil {
			return err
		}
		if err := validateHostBlockAlias(host.Alias); err != nil {
			return err
		}
	}
	return nil
}
