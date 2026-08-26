//go:build !windows && !linux

package storage

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// O_PATHがないUnixでは従来どおりreadable directory descriptorを使う。
func walkDirectoryFlags(bool) int {
	return unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_DIRECTORY
}

func openWalkRoot() (*os.File, error) {
	fd, err := unix.Open(string(filepath.Separator), walkDirectoryFlags(false), 0)
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
