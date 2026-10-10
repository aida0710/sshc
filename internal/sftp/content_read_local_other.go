//go:build !unix

package sftp

import "os"

func openLocalContentFile(root *os.Root, relative string) (*os.File, error) {
	return root.Open(relative)
}
