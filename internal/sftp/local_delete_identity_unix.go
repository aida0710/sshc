//go:build unix

package sftp

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

func localDeletionIdentity(_ *os.Root, _ string, metadata fs.FileInfo) (string, error) {
	stat, ok := metadata.Sys().(*syscall.Stat_t)
	if !ok {
		return "", ErrUnsupportedEntry
	}
	return fmt.Sprintf("%d:%d", stat.Dev, stat.Ino), nil
}
