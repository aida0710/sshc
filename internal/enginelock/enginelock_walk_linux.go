//go:build linux

package enginelock

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// Android の SELinux domain は既知のアプリ専用パスを通過できても、/ や /data を
// directory として読むことはできない。中間 descriptor は O_PATH で lookup 専用にし、
// lock file を作る最終 directory だけを読み書き可能な descriptor として開く。
func lockWalkDirectoryFlags(readable bool) int {
	flags := unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_DIRECTORY
	if readable {
		return flags | unix.O_RDONLY
	}
	return flags | unix.O_PATH
}

func openLockWalkRoot() (*os.File, error) {
	fd, err := unix.Open(string(filepath.Separator), lockWalkDirectoryFlags(false), 0)
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
