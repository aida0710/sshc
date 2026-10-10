//go:build linux

package sftp

import (
	"io/fs"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func captureLocalTextMetadata(file *os.File) (localTextMetadata, error) {
	info, err := file.Stat()
	if err != nil {
		return localTextMetadata{}, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() {
		return localTextMetadata{}, ErrUnsupportedEntry
	}
	attributes, err := readLocalTextExtendedAttributes(file)
	if err != nil {
		return localTextMetadata{}, err
	}
	metadata := localTextMetadata{uid: int(stat.Uid), gid: int(stat.Gid), mode: info.Mode(), extendedAttributes: attributes}
	metadata.revision = localTextMetadataRevision(metadata)
	return metadata, nil
}

func applyLocalTextMetadata(_ *os.File, staged *os.File, snapshot localTextMetadata) error {
	// Privileged executable bits and capabilities need a deliberate policy when
	// changing contents. The text editor only replaces ordinary unprivileged files.
	if snapshot.mode&(fs.ModeSetuid|fs.ModeSetgid|fs.ModeSticky) != 0 {
		return ErrUnsupportedEntry
	}
	if _, privileged := snapshot.extendedAttributes["security.capability"]; privileged {
		return ErrUnsupportedEntry
	}
	current, err := captureLocalTextMetadata(staged)
	if err != nil {
		return err
	}
	if current.uid != snapshot.uid || current.gid != snapshot.gid {
		if err := staged.Chown(snapshot.uid, snapshot.gid); err != nil {
			return err
		}
	}
	fd := int(staged.Fd())
	// A directory's inherited ACL must not broaden the replacement's access.
	for name := range current.extendedAttributes {
		if _, retain := snapshot.extendedAttributes[name]; !retain {
			if err := unix.Fremovexattr(fd, name); err != nil {
				return err
			}
		}
	}
	if err := staged.Chmod(snapshot.mode.Perm()); err != nil {
		return err
	}
	for name, value := range snapshot.extendedAttributes {
		if err := unix.Fsetxattr(fd, name, value, 0); err != nil {
			return err
		}
	}
	return nil
}
