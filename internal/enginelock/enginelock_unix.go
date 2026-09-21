//go:build unix

package enginelock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sshc/internal/platform/nofollow"
	"syscall"

	"golang.org/x/sys/unix"
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

// openOrCreateLockDirectory は lock file の親を symlink をたどらずに開く。歩き方は
// ワークスペースの他の非公開ディレクトリと同じで、途中の symlink や通常ファイルは
// この engine の state directory として信用しない。
func openOrCreateLockDirectory(path string) (*os.File, error) {
	directory, err := nofollow.OpenOrCreateDirectory(path, 0o700)
	if errors.Is(err, nofollow.ErrSymlinkPath) || errors.Is(err, unix.ENOTDIR) {
		return nil, fmt.Errorf("%w: %s", ErrUnsafeStateDirectory, path)
	}
	return directory, err
}
