//go:build linux || darwin

package sftp

import (
	"os"

	"golang.org/x/sys/unix"
)

func renameLocalTextStagingContents(parent *os.Root, staging localTextStaging, target string) error {
	sourceDirectory, err := staging.directory.Open(".")
	if err != nil {
		return err
	}
	defer sourceDirectory.Close()
	targetDirectory, err := parent.Open(".")
	if err != nil {
		return err
	}
	defer targetDirectory.Close()
	// Keep both directory identities fixed even if a parent entry changes
	// after verification. Only the final destination entry is replaced.
	return unix.Renameat(int(sourceDirectory.Fd()), localTextStagingContentsName, int(targetDirectory.Fd()), target)
}
