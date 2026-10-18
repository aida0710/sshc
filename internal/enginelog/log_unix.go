//go:build !windows

package enginelog

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
	"sshc/internal/platform/nofollow"
	"sshc/internal/storage"
)

func openPrivateAppend(path string) (*os.File, error) {
	directory, err := nofollow.OpenOrCreateDirectory(filepath.Dir(path), storage.DirectoryPermission)
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	descriptor, err := unix.Openat(int(directory.Fd()), filepath.Base(path), unix.O_WRONLY|unix.O_CREAT|unix.O_APPEND|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, uint32(storage.FilePermission))
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(descriptor), path)
	info, err := file.Stat()
	if err == nil && !info.Mode().IsRegular() {
		err = storage.ErrNotRegularFile
	}
	if err == nil {
		err = file.Chmod(storage.FilePermission)
	}
	if err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}

func openPrivateRead(path string) (*os.File, error) { return nofollow.OpenRegular(path) }
