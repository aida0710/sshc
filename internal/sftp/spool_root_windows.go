//go:build windows

package sftp

import (
	"io/fs"
	"os"
)

// prepareDownloadSpoolRoot creates the spool root when it is missing. The
// root sits in the user's profile, which other accounts cannot write, and
// each spool directory and the quota lock in it open with an ACL that admits
// only the current user. A link in place of the root is refused so the spool
// cannot be redirected elsewhere.
func prepareDownloadSpoolRoot(spoolRoot string) error {
	if err := os.MkdirAll(spoolRoot, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(spoolRoot)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
		return errSpoolRootNotPrivate
	}
	return nil
}
