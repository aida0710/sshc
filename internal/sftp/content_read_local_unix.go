//go:build unix

package sftp

import (
	"os"

	"golang.org/x/sys/unix"
)

// A regular file can be replaced by a FIFO after Lstat. Opening without waiting
// lets the existing handle/type checks reject it before any content is read.
func openLocalContentFile(root *os.Root, relative string) (*os.File, error) {
	return root.OpenFile(relative, os.O_RDONLY|unix.O_NONBLOCK, 0)
}
