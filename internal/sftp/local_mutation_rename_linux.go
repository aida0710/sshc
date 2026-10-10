//go:build linux && !android

package sftp

import (
	"os"

	"golang.org/x/sys/unix"
)

func renameLocalMutationInDirectory(directory *os.File, from, to string) error {
	descriptor := int(directory.Fd())
	return unix.Renameat2(descriptor, from, descriptor, to, unix.RENAME_NOREPLACE)
}
