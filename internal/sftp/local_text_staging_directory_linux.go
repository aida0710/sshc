package sftp

import (
	"os"

	"golang.org/x/sys/unix"
)

const (
	localTextAccessACLAttribute  = "system.posix_acl_access"
	localTextDefaultACLAttribute = "system.posix_acl_default"
)

func secureLocalTextStagingDirectory(directory *os.File) error {
	if _, err := localTextStagingDirectoryInfo(directory); err != nil {
		return err
	}
	attributes, err := readLocalTextExtendedAttributes(directory)
	if err != nil {
		return err
	}
	for _, name := range []string{localTextAccessACLAttribute, localTextDefaultACLAttribute} {
		if _, exists := attributes[name]; exists {
			if err := unix.Fremovexattr(int(directory.Fd()), name); err != nil {
				return err
			}
		}
	}
	if err := directory.Chmod(localTextStagingDirectoryPermission); err != nil {
		return err
	}
	return verifyLocalTextStagingDirectory(directory)
}

func verifyLocalTextStagingDirectory(directory *os.File) error {
	info, err := localTextStagingDirectoryInfo(directory)
	if err != nil {
		return err
	}
	if info.Mode().Perm() != localTextStagingDirectoryPermission {
		return ErrConflict
	}
	attributes, err := readLocalTextExtendedAttributes(directory)
	if err != nil {
		return err
	}
	for _, name := range []string{localTextAccessACLAttribute, localTextDefaultACLAttribute} {
		if _, exists := attributes[name]; exists {
			return ErrConflict
		}
	}
	return nil
}
