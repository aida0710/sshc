//go:build windows

package sftp

import (
	"encoding/binary"
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

func holdDownloadSpoolQuota(temporaryRoot string) (io.Closer, error) {
	file, err := windowsacl.OpenOrCreateFile(filepath.Join(temporaryRoot, ".sshc-sftp-spool-quota.lock"))
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

func readDownloadSpoolReservation(directory string) (int64, error) {
	file, err := windowsacl.OpenAuthenticatedFileForRead(filepath.Join(directory, ".reserved"))
	if err != nil {
		return 0, err
	}
	defer file.Close()
	var encoded [8]byte
	if _, err := io.ReadFull(file, encoded[:]); err != nil {
		return 0, err
	}
	var extra [1]byte
	if count, err := file.Read(extra[:]); err != io.EOF || count != 0 {
		if err != nil {
			return 0, err
		}
		return 0, os.ErrInvalid
	}
	value := binary.BigEndian.Uint64(encoded[:])
	if value > uint64(maxProcessDownloadSpoolBytes) {
		return 0, os.ErrInvalid
	}
	return int64(value), nil
}

func writeDownloadSpoolReservation(directory string, reserved int64) error {
	if reserved < 0 || reserved > maxProcessDownloadSpoolBytes {
		return os.ErrInvalid
	}
	file, err := windowsacl.OpenOrCreateFile(filepath.Join(directory, ".reserved"))
	if err != nil {
		return err
	}
	defer file.Close()
	if err := file.Truncate(0); err != nil {
		return err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], uint64(reserved))
	if _, err := file.Write(encoded[:]); err != nil {
		return err
	}
	return file.Sync()
}
