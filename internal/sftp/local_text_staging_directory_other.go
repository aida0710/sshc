//go:build !linux && !darwin && !windows

package sftp

import "os"

func secureLocalTextStagingDirectory(_ *os.File) error {
	return ErrUnsupportedOperation
}

func verifyLocalTextStagingDirectory(_ *os.File) error {
	return ErrUnsupportedOperation
}
