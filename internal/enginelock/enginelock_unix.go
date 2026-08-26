//go:build unix

package enginelock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

	"sshc/internal/platform/nativepath"
)

var afterLockDirectoryOpen func()

// acquire は flock(LOCK_EX|LOCK_NB) を使う。open file description ごとに効くので、
// 同じプロセスが開き直した 2 本目でも、別プロセスと同じように弾かれる。
func acquire(path string) (func() error, error) {
	directory := filepath.Dir(path)
	parent, err := openOrCreateLockDirectory(directory)
	if err != nil {
		return nil, err
	}
	closeParent := true
	defer func() {
		if closeParent {
			_ = parent.Close()
		}
	}()
	if afterLockDirectoryOpen != nil {
		afterLockDirectoryOpen()
	}
	// The directory descriptor is the workspace-lock identity. Resolve and create
	// the file relative to it, so renaming or replacing the pathname after the
	// directory check cannot redirect this acquisition to another inode.
	descriptor, err := unix.Openat(int(parent.Fd()), filepath.Base(path), unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(descriptor), path)
	// 既にあるファイルには OpenFile の perm が効かない。開いた fd 越しに直すので、
	// パスを取り違える隙が無い。
	if err := file.Chmod(0o600); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	if lockErr := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); lockErr != nil {
		closeErr := file.Close()
		if errors.Is(lockErr, syscall.EWOULDBLOCK) {
			lockErr = ErrRunning
		}
		return nil, errors.Join(lockErr, closeErr)
	}
	lockedDescriptor := int(file.Fd())
	closeParent = false
	return newReleaseWithDirectory(file, parent,
		func() error { return syscall.Flock(lockedDescriptor, syscall.LOCK_UN) }), nil
}

func openOrCreateLockDirectory(path string) (*os.File, error) {
	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) {
		return nil, os.ErrInvalid
	}
	cleaned, err := nativepath.ResolveRootAlias(cleaned)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrUnsafeStateDirectory, path, err)
	}
	current, err := openLockWalkRoot()
	if err != nil {
		return nil, err
	}
	components := strings.Split(strings.TrimPrefix(cleaned, string(filepath.Separator)), string(filepath.Separator))
	for index, component := range components {
		if component == "" || component == "." || component == ".." {
			_ = current.Close()
			return nil, os.ErrInvalid
		}
		final := index == len(components)-1
		fd, openErr := unix.Openat(int(current.Fd()), component, lockWalkDirectoryFlags(final), 0)
		if errors.Is(openErr, unix.ENOENT) {
			if mkdirErr := unix.Mkdirat(int(current.Fd()), component, 0o700); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
				_ = current.Close()
				return nil, mkdirErr
			}
			fd, openErr = unix.Openat(int(current.Fd()), component, lockWalkDirectoryFlags(final), 0)
		}
		if openErr != nil {
			_ = current.Close()
			if errors.Is(openErr, unix.ELOOP) || errors.Is(openErr, unix.ENOTDIR) {
				return nil, fmt.Errorf("%w: %s", ErrUnsafeStateDirectory, path)
			}
			return nil, openErr
		}
		next := os.NewFile(uintptr(fd), component)
		if next == nil {
			_ = unix.Close(fd)
			_ = current.Close()
			return nil, os.ErrInvalid
		}
		_ = current.Close()
		current = next
		if index == len(components)-1 {
			if err := current.Chmod(0o700); err != nil {
				_ = current.Close()
				return nil, err
			}
		}
	}
	return current, nil
}
