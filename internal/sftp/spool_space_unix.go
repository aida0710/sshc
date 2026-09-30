//go:build !windows

package sftp

import (
	"errors"

	"golang.org/x/sys/unix"
)

// spoolFreeBytes is the space this user may still write in directory's
// filesystem. A tmpfs reports its own size limit here.
func spoolFreeBytes(directory string) (uint64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(directory, &stat); err != nil {
		return 0, err
	}
	return uint64(stat.Bavail) * uint64(stat.Bsize), nil
}

func isNoSpaceLeft(err error) bool {
	return errors.Is(err, unix.ENOSPC) || errors.Is(err, unix.EDQUOT)
}
