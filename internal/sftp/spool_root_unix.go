//go:build !windows

package sftp

import "os"

// prepareDownloadSpoolRoot creates the spool root when it is missing and
// refuses one that another account could write. The quota lock in the root
// has a predictable name, so a writable root would let another local account
// create the lock first and block every download.
func prepareDownloadSpoolRoot(spoolRoot string) error {
	if err := os.MkdirAll(spoolRoot, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(spoolRoot)
	if err != nil {
		return err
	}
	if info.IsDir() && info.Mode().Perm() != 0o700 {
		// MkdirAll keeps the mode of a directory that already existed.
		if err := os.Chmod(spoolRoot, 0o700); err != nil {
			return err
		}
		if info, err = os.Lstat(spoolRoot); err != nil {
			return err
		}
	}
	if !info.IsDir() || !downloadSpoolDirectoryTrusted(info, spoolRoot) {
		return errSpoolRootNotPrivate
	}
	return nil
}
