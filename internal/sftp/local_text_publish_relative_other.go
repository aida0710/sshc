//go:build !linux && !darwin && !windows

package sftp

import "os"

func renameLocalTextStagingContents(_ *os.Root, _ localTextStaging, _ string) error {
	return ErrUnsupportedOperation
}
