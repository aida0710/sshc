package sftp

import (
	"errors"
	"io/fs"
	"os"
)

// publishLocalWithoutReplace makes a finished download visible as target
// unless something already exists there. A hard link publishes atomically on
// the filesystems that support it; FAT, exFAT and some SMB mounts cannot make
// one, so they fall back to a rename that refuses an existing target.
func publishLocalWithoutReplace(root *os.Root, temporary, target string) error {
	return localLink((*os.Root).Link).publishWithoutReplace(root, temporary, target)
}

// localLink makes newname a hard link to oldname under root. Tests stand in
// one that fails the way a filesystem without hard links does.
type localLink func(root *os.Root, oldname, newname string) error

// publishWithoutReplace is publishLocalWithoutReplace with link making the
// hard link.
func (link localLink) publishWithoutReplace(root *os.Root, temporary, target string) error {
	err := link(root, temporary, target)
	if err == nil {
		return nil
	}
	if errors.Is(err, fs.ErrExist) {
		return ErrAlreadyExists
	}
	return renameLocalWithoutReplace(root, temporary, target)
}

// renameLocalAfterCheck is the last resort where no atomic rename that refuses
// an existing target is available. Something created between the check and
// the rename is replaced, the same window the check before publish leaves.
func renameLocalAfterCheck(root *os.Root, temporary, target string) error {
	current, err := checkLocal(root, target, true)
	if err != nil {
		return err
	}
	if current != nil {
		return ErrAlreadyExists
	}
	return root.Rename(temporary, target)
}
