//go:build !windows && !darwin

package sftp

import "os"

// Staged contents stay private until their original access policy is restored.
const localTextStagingPermission = 0o600

func openLocalTextStagingFile(parent *os.Root, name string) (*os.File, error) {
	return parent.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, localTextStagingPermission)
}

func publishLocalTextReplacement(parent *os.Root, temporary, target string) error {
	return parent.Rename(temporary, target)
}
