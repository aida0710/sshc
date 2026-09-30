//go:build !linux || android

package sftp

import "os"

// renameLocalWithoutReplace checks before renaming. macOS and Windows have
// their own renames that refuse an existing target, but os.Root exposes
// neither. Android could call renameat2, but see local_publish_linux.go for
// why it does not.
func renameLocalWithoutReplace(root *os.Root, temporary, target string) error {
	return renameLocalAfterCheck(root, temporary, target)
}
