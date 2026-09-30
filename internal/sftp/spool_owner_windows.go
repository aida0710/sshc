//go:build windows

package sftp

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"golang.org/x/sys/windows"

	"sshc/internal/platform/windowsacl"
)

type windowsSpoolOwner struct {
	file       *os.File
	overlapped windows.Overlapped
}

func (owner *windowsSpoolOwner) Close() error {
	handle := windows.Handle(owner.file.Fd())
	unlockErr := windows.UnlockFileEx(handle, 0, 1, 0, &owner.overlapped)
	runtime.KeepAlive(owner.file)
	return errors.Join(unlockErr, owner.file.Close())
}

func prepareDownloadSpoolDirectory(path string) error {
	return windowsacl.RestrictDirectory(path)
}

func downloadSpoolDirectoryTrusted(info fs.FileInfo, path string) bool {
	if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
		return false
	}
	restricted, err := windowsacl.IsRestrictedToCurrentUser(path)
	return err == nil && restricted
}

func lockSpoolOwner(file *os.File, overlapped *windows.Overlapped) error {
	handle := windows.Handle(file.Fd())
	err := windows.LockFileEx(
		handle,
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1,
		0,
		overlapped,
	)
	runtime.KeepAlive(file)
	return err
}

func holdDownloadSpoolOwner(path string) (io.Closer, error) {
	file, err := windowsacl.OpenOrCreateFile(filepath.Join(path, ".owner.lock"))
	if err != nil {
		return nil, err
	}
	owner := &windowsSpoolOwner{file: file}
	if err := lockSpoolOwner(file, &owner.overlapped); err != nil {
		_ = file.Close()
		return nil, err
	}
	return owner, nil
}

func downloadSpoolOwnerState(path string) (managed, inactive bool, resultErr error) {
	ownerPath := filepath.Join(path, ".owner.lock")
	if _, err := os.Lstat(ownerPath); errors.Is(err, os.ErrNotExist) {
		return false, false, nil
	} else if err != nil {
		return true, false, err
	}
	file, err := windowsacl.OpenAuthenticatedFile(ownerPath)
	if err != nil {
		return true, false, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, file.Close())
	}()
	var overlapped windows.Overlapped
	if err := lockSpoolOwner(file, &overlapped); err != nil {
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return true, false, nil
		}
		return true, false, err
	}
	handle := windows.Handle(file.Fd())
	unlockErr := windows.UnlockFileEx(handle, 0, 1, 0, &overlapped)
	runtime.KeepAlive(file)
	return true, true, unlockErr
}

func holdDownloadSpoolQuota(spoolRoot string) (io.Closer, error) {
	file, err := windowsacl.OpenOrCreateFile(filepath.Join(spoolRoot, ".sshc-sftp-spool-quota.lock"))
	if err != nil {
		return nil, err
	}
	owner := &windowsSpoolOwner{file: file}
	handle := windows.Handle(file.Fd())
	err = windows.LockFileEx(handle, windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &owner.overlapped)
	runtime.KeepAlive(file)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	return owner, nil
}

// openSpoolFileForRead and openOrCreateSpoolFile open a file of the spool
// that only this user may read or write. The reservation format itself is in
// spool_reservation.go.
func openSpoolFileForRead(path string) (*os.File, error) {
	return windowsacl.OpenAuthenticatedFileForRead(path)
}

func openOrCreateSpoolFile(path string) (*os.File, error) {
	return windowsacl.OpenOrCreateFile(path)
}
