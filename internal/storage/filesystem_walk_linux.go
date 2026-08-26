//go:build linux

package storage

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// walkDirectoryFlags は中間directoryをlookup専用descriptorとして開く。
// Android 16ではappが既知のprivate pathを通過できても、/ や /dataの一覧を
// 読む権限はない。O_PATHならread権限を要求せず、openatのanchorとしてだけ使える。
func walkDirectoryFlags(readable bool) int {
	flags := unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_DIRECTORY
	if readable {
		return flags | unix.O_RDONLY
	}
	return flags | unix.O_PATH
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
