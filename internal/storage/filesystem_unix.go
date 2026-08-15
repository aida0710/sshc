//go:build !windows

package storage

import (
	"errors"
	"io/fs"
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

func makePrivateDirectories(path string, permission fs.FileMode) error {
	if err := os.MkdirAll(path, permission); err != nil {
		return err
	}
	return os.Chmod(path, permission)
}

func createPrivateTemp(directory, prefix string) (*os.File, error) {
	return os.CreateTemp(directory, prefix)
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
