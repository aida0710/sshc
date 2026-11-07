//go:build !windows

package privatefile

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
	"sshc/internal/platform/nofollow"
	"sshc/internal/storage"
)

// OpenAppend は、path を本人だけが読み書きできるファイルとして、追記で開く。無ければ作る。途中のフォルダも
// 本人だけのものとして作る。
func OpenAppend(path string) (*os.File, error) {
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

// OpenRead は、path をリンクをたどらずに読むために開く。
func OpenRead(path string) (*os.File, error) { return nofollow.OpenRegular(path) }
