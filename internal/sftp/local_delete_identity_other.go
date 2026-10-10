//go:build !unix && !windows

package sftp

import (
	"io/fs"
	"os"
)

func localDeletionIdentity(_ *os.Root, _ string, _ fs.FileInfo) (string, error) {
	return "", ErrUnsupportedEntry
}
