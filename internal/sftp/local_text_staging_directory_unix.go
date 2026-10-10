//go:build linux || darwin

package sftp

import (
	"io/fs"
	"os"
	"syscall"
)

func localTextStagingDirectoryInfo(directory *os.File) (fs.FileInfo, error) {
	info, err := directory.Stat()
	if err != nil {
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || int(stat.Uid) != os.Geteuid() {
		return nil, ErrConflict
	}
	return info, nil
}
