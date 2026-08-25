//go:build !windows

package sftp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

type unixSpoolOwner struct {
	file *os.File
}

func (owner *unixSpoolOwner) Close() error {
	return errors.Join(
		unix.Flock(int(owner.file.Fd()), unix.LOCK_UN),
		owner.file.Close(),
	)
}

func prepareDownloadSpoolDirectory(path string) error {
	return os.Chmod(path, 0o700)
}

func downloadSpoolDirectoryTrusted(info fs.FileInfo, _ string) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Getuid()) && info.Mode().Perm() == 0o700
}

func openSpoolOwner(path string) (*os.File, error) {
	ownerPath := filepath.Join(path, ".owner.lock")
	return openUnixPrivateFile(ownerPath, unix.O_CREAT|unix.O_RDWR)
}

func openUnixPrivateFile(path string, flags int) (*os.File, error) {
	fd, err := unix.Open(path, flags|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG ||
		stat.Uid != uint32(os.Getuid()) || stat.Mode&0o077 != 0 {
		_ = unix.Close(fd)
		if err != nil {
			return nil, err
		}
		return nil, os.ErrPermission
	}
	return os.NewFile(uintptr(fd), path), nil
}

func holdDownloadSpoolOwner(path string) (io.Closer, error) {
	file, err := openSpoolOwner(path)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, err
	}
	return &unixSpoolOwner{file: file}, nil
}

func downloadSpoolOwnerState(path string) (managed, inactive bool, resultErr error) {
	ownerPath := filepath.Join(path, ".owner.lock")
	if _, err := os.Lstat(ownerPath); errors.Is(err, os.ErrNotExist) {
		return false, false, nil
	} else if err != nil {
		return true, false, err
	}
	file, err := openSpoolOwner(path)
	if err != nil {
		return true, false, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, file.Close())
	}()
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return true, false, nil
		}
		return true, false, err
	}
	return true, true, unix.Flock(int(file.Fd()), unix.LOCK_UN)
}

func holdDownloadSpoolQuota(temporaryRoot string) (io.Closer, error) {
	quotaPath := filepath.Join(temporaryRoot, fmt.Sprintf(".sshc-sftp-spool-quota-%d.lock", os.Getuid()))
	file, err := openUnixPrivateFile(quotaPath, unix.O_CREAT|unix.O_RDWR)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, err
	}
	return &unixSpoolOwner{file: file}, nil
}

func readDownloadSpoolReservation(directory string) (int64, error) {
	file, err := openUnixPrivateFile(filepath.Join(directory, ".reserved"), unix.O_RDONLY)
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
	file, err := openUnixPrivateFile(filepath.Join(directory, ".reserved"), unix.O_CREAT|unix.O_RDWR)
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
