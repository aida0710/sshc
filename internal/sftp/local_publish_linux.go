//go:build linux && !android

package sftp

import (
	"errors"
	"os"
	"path"

	"golang.org/x/sys/unix"
)

// renameLocalWithoutReplace asks the kernel to refuse an existing target, so
// nothing can appear between a check and the rename. vfat and exfat support
// RENAME_NOREPLACE; a filesystem or kernel without it falls back to a check.
// Android is left out: its app seccomp filter kills the process with SIGSYS
// instead of answering ENOSYS for a system call it does not allow, and
// whether minSdk 26 allows renameat2 has not been confirmed.
func renameLocalWithoutReplace(root *os.Root, temporary, target string) error {
	directory := path.Dir(target)
	if path.Dir(temporary) != directory {
		return renameLocalAfterCheck(root, temporary, target)
	}
	parent, err := root.Open(directory)
	if err != nil {
		return err
	}
	defer parent.Close()
	descriptor := int(parent.Fd())
	err = unix.Renameat2(descriptor, path.Base(temporary), descriptor, path.Base(target), unix.RENAME_NOREPLACE)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, unix.EEXIST):
		return ErrAlreadyExists
	case errors.Is(err, unix.EINVAL), errors.Is(err, unix.ENOSYS):
		return renameLocalAfterCheck(root, temporary, target)
	}
	return &os.LinkError{Op: "renameat2", Old: temporary, New: target, Err: err}
}
