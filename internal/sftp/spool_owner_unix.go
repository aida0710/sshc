//go:build !windows

package sftp

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

func prepareDownloadSpoolDirectory(path string) error {
	return os.Chmod(path, 0o700)
}

func downloadSpoolDirectoryTrusted(info fs.FileInfo, _ string) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Getuid()) && info.Mode().Perm() == 0o700
}

func openUnixPrivateFile(path string, flags int) (*os.File, error) {
	fd, err := unix.Open(path, flags|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG ||
		stat.Uid != uint32(os.Getuid()) || stat.Mode&0o077 != 0 {
		_ = unix.Close(fd)
		if err != nil {
			return nil, err
		}
		return nil, os.ErrPermission
	}
	return os.NewFile(uintptr(fd), path), nil
}

// downloadSpoolQuotaPath is the lock every sshc process of this user takes
// before it counts the spools under spoolRoot.
func downloadSpoolQuotaPath(spoolRoot string) string {
	return filepath.Join(spoolRoot, fmt.Sprintf(".sshc-sftp-spool-quota-%d.lock", os.Getuid()))
}

// openSpoolFileForRead and openOrCreateSpoolFile open a file of the spool
// that only this user may read or write. The reservation format itself is in
// spool_reservation.go.
func openSpoolFileForRead(path string) (*os.File, error) {
	return openUnixPrivateFile(path, unix.O_RDONLY)
}

func openOrCreateSpoolFile(path string) (*os.File, error) {
	return openUnixPrivateFile(path, unix.O_CREAT|unix.O_RDWR)
}
