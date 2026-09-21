//go:build !windows

package storage

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sshc/internal/platform/nofollow"
	"strings"

	"golang.org/x/sys/unix"
)

const (
	tempRandomByteCount = 16
	tempCollisionLimit  = 128
)

func openRegularNoFollow(path string) (*os.File, error) {
	return nofollow.OpenRegular(path)
}

func openDirectoryNoFollow(path string) (*os.File, error) {
	return nofollow.OpenDirectory(path)
}

func makePrivateDirectories(path string, permission fs.FileMode) error {
	directory, err := nofollow.OpenOrCreateDirectory(path, permission)
	if err != nil {
		return err
	}
	return directory.Close()
}

func createPrivateTemp(directory, prefix string) (*os.File, error) {
	parent, err := openDirectoryNoFollow(directory)
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	return createPrivateTempAt(parent, directory, prefix)
}

func createPrivateTempAt(parent *os.File, directory, prefix string) (*os.File, error) {
	if prefix != filepath.Base(prefix) || strings.ContainsRune(prefix, filepath.Separator) {
		return nil, os.ErrInvalid
	}
	randomBytes := make([]byte, tempRandomByteCount)
	defer clear(randomBytes)
	for attempt := 0; attempt < tempCollisionLimit; attempt++ {
		if _, err := rand.Read(randomBytes); err != nil {
			return nil, err
		}
		name := prefix + hex.EncodeToString(randomBytes)
		fd, err := unix.Openat(
			int(parent.Fd()), name,
			unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW,
			uint32(FilePermission),
		)
		if errors.Is(err, unix.EEXIST) {
			continue
		}
		if err != nil {
			return nil, err
		}
		file := os.NewFile(uintptr(fd), filepath.Join(directory, name))
		if file == nil {
			_ = unix.Close(fd)
			_ = unix.Unlinkat(int(parent.Fd()), name, 0)
			return nil, os.ErrInvalid
		}
		return file, nil
	}
	return nil, fmt.Errorf("create private temp: collision limit exceeded")
}

func replaceFile(oldPath, newPath string) error {
	oldDirectory := filepath.Dir(filepath.Clean(oldPath))
	newDirectory := filepath.Dir(filepath.Clean(newPath))
	oldParent, err := openDirectoryNoFollow(oldDirectory)
	if err != nil {
		return err
	}
	defer oldParent.Close()
	newParent := oldParent
	if oldDirectory != newDirectory {
		newParent, err = openDirectoryNoFollow(newDirectory)
		if err != nil {
			return err
		}
		defer newParent.Close()
	}
	return unix.Renameat(int(oldParent.Fd()), filepath.Base(oldPath), int(newParent.Fd()), filepath.Base(newPath))
}

func movePrivateFile(oldPath, newPath string) error { return replaceFile(oldPath, newPath) }

func removeNoFollow(path string) error {
	cleaned := filepath.Clean(path)
	parent, err := openDirectoryNoFollow(filepath.Dir(cleaned))
	if err != nil {
		return err
	}
	defer parent.Close()
	name := filepath.Base(cleaned)
	var stat unix.Stat_t
	if err := unix.Fstatat(int(parent.Fd()), name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	flags := 0
	if stat.Mode&unix.S_IFMT == unix.S_IFDIR {
		flags = unix.AT_REMOVEDIR
	}
	return unix.Unlinkat(int(parent.Fd()), name, flags)
}

func syncDirectory(path string) error {
	directory, err := openDirectoryNoFollow(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func writeAtomicFileNative(path, prefix string, permission fs.FileMode, contents []byte) error {
	return writeAtomicFileNativeWith(path, prefix, permission, contents, nil)
}

func writeAtomicFileNativeWith(path, prefix string, permission fs.FileMode, contents []byte, afterParentOpen func()) error {
	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) || filepath.Base(cleaned) == "." {
		return os.ErrInvalid
	}
	directory := filepath.Dir(cleaned)
	parent, err := openDirectoryNoFollow(directory)
	if err != nil {
		return err
	}
	defer parent.Close()
	if afterParentOpen != nil {
		afterParentOpen()
	}
	temporary, err := createPrivateTempAt(parent, directory, prefix)
	if err != nil {
		return err
	}
	temporaryName := filepath.Base(temporary.Name())
	removeTemporary := true
	defer func() {
		_ = temporary.Close()
		if removeTemporary {
			_ = unix.Unlinkat(int(parent.Fd()), temporaryName, 0)
		}
	}()
	if err := writeAndFlush(temporary, permission, contents); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := unix.Renameat(int(parent.Fd()), temporaryName, int(parent.Fd()), filepath.Base(cleaned)); err != nil {
		return err
	}
	removeTemporary = false
	return parent.Sync()
}
