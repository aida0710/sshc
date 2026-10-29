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
	return statfsFreeBytes(&stat), nil
}

// availableBlocks reads a free-block count whose type differs by OS. FreeBSD
// and OpenBSD count it signed, and it goes below zero once the root reserve is
// in use.
func availableBlocks[Count int64 | uint64](count Count) uint64 {
	if count < 0 {
		return 0
	}
	return uint64(count)
}

func isNoSpaceLeft(err error) bool {
	return errors.Is(err, unix.ENOSPC) || errors.Is(err, unix.EDQUOT)
}
