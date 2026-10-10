//go:build linux || darwin

package sftp

import (
	"bytes"
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// Chown and Chmod can change ACL masks or other attributes. Read the current
// values afterward so unchanged system labels do not need write permission.
func applyLocalTextExtendedAttributes(file *os.File, attributes map[string][]byte) error {
	current, err := readLocalTextExtendedAttributes(file)
	if err != nil {
		return err
	}
	fd := int(file.Fd())
	for name := range current {
		if _, retain := attributes[name]; !retain {
			if err := unix.Fremovexattr(fd, name); err != nil {
				return err
			}
		}
	}
	for name, value := range attributes {
		if existing, exists := current[name]; exists && bytes.Equal(existing, value) {
			continue
		}
		if err := unix.Fsetxattr(fd, name, value, 0); err != nil {
			return err
		}
	}
	return nil
}

func readLocalTextExtendedAttributes(file *os.File) (map[string][]byte, error) {
	fd := int(file.Fd())
	nameBytes, err := unix.Flistxattr(fd, nil)
	if errors.Is(err, unix.ENOTSUP) {
		return map[string][]byte{}, nil
	}
	if err != nil {
		return nil, err
	}
	if nameBytes < 0 || nameBytes > maxLocalTextMetadataBytes {
		return nil, ErrUnsupportedEntry
	}
	if nameBytes == 0 {
		return map[string][]byte{}, nil
	}
	names := make([]byte, nameBytes)
	read, err := unix.Flistxattr(fd, names)
	if err != nil {
		return nil, err
	}
	if read < 0 || read > len(names) || read == 0 || names[read-1] != 0 {
		return nil, ErrConflict
	}
	attributes := make(map[string][]byte)
	retainedBytes := read
	for _, encoded := range bytes.Split(names[:read-1], []byte{0}) {
		if len(encoded) == 0 {
			return nil, ErrConflict
		}
		name := string(encoded)
		size, err := unix.Fgetxattr(fd, name, nil)
		if err != nil {
			return nil, err
		}
		if size < 0 || size > maxLocalTextMetadataBytes-retainedBytes {
			return nil, ErrUnsupportedEntry
		}
		value := make([]byte, size)
		read, err := unix.Fgetxattr(fd, name, value)
		if err != nil {
			return nil, err
		}
		if read != size {
			return nil, ErrConflict
		}
		attributes[name] = value
		retainedBytes += size
	}
	return attributes, nil
}
