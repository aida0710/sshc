//go:build !linux && !darwin && !windows

package sftp

import "os"

func captureLocalTextMetadata(_ *os.File) (localTextMetadata, error) {
	return localTextMetadata{}, ErrUnsupportedOperation
}

func applyLocalTextMetadata(_, _ *os.File, _ localTextMetadata) error {
	return ErrUnsupportedOperation
}
