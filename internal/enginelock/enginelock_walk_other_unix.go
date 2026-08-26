//go:build unix && !linux

package enginelock

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func lockWalkDirectoryFlags(bool) int {
	return unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_DIRECTORY
}

func openLockWalkRoot() (*os.File, error) {
	fd, err := unix.Open(string(filepath.Separator), lockWalkDirectoryFlags(true), 0)
	if err != nil {
		return nil, err
	}
	root := os.NewFile(uintptr(fd), string(filepath.Separator))
	if root == nil {
		_ = unix.Close(fd)
		return nil, os.ErrInvalid
	}
	return root, nil
}
