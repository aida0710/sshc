//go:build windows

package sftp

import (
	"io/fs"
	"os"
	"path/filepath"

	"sshc/internal/platform/windowsacl"
)

func prepareDownloadSpoolDirectory(path string) error {
	return windowsacl.RestrictDirectory(path)
}

func downloadSpoolDirectoryTrusted(info fs.FileInfo, path string) bool {
	if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
		return false
	}
	restricted, err := windowsacl.IsRestrictedToCurrentUser(path)
	return err == nil && restricted
}

// downloadSpoolQuotaPath is the lock every sshc process of this user takes
// before it counts the spools under spoolRoot.
func downloadSpoolQuotaPath(spoolRoot string) string {
	return filepath.Join(spoolRoot, ".sshc-sftp-spool-quota.lock")
}

// openSpoolFileForRead and openOrCreateSpoolFile open a file of the spool
// that only this user may read or write. The reservation format itself is in
// spool_reservation.go.
func openSpoolFileForRead(path string) (*os.File, error) {
	return windowsacl.OpenAuthenticatedFileForRead(path)
}

func openOrCreateSpoolFile(path string) (*os.File, error) {
	return windowsacl.OpenOrCreateFile(path)
}
