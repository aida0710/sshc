//go:build !windows

package storage

import (
	"errors"
	"os"
	"syscall"
)

func openRegularNoFollow(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if errors.Is(err, syscall.ELOOP) {
		return nil, ErrSymlinkPath
	}
	return file, err
}

func replaceFile(oldPath, newPath string) error { return os.Rename(oldPath, newPath) }

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func restrictPrivatePath(path string, directory bool) error {
	permission := FilePermission
	if directory {
		permission = DirectoryPermission
	}
	return os.Chmod(path, permission)
}
