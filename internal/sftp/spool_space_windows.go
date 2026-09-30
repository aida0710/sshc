package sftp

import (
	"errors"

	"golang.org/x/sys/windows"
)

// spoolFreeBytes is the space this user may still write in directory's
// volume, after any disk quota.
func spoolFreeBytes(directory string) (uint64, error) {
	name, err := windows.UTF16PtrFromString(directory)
	if err != nil {
		return 0, err
	}
	var available, total, free uint64
	if err := windows.GetDiskFreeSpaceEx(name, &available, &total, &free); err != nil {
		return 0, err
	}
	return available, nil
}

func isNoSpaceLeft(err error) bool {
	return errors.Is(err, windows.ERROR_DISK_FULL) || errors.Is(err, windows.ERROR_HANDLE_DISK_FULL) ||
		errors.Is(err, windows.ERROR_DISK_QUOTA_EXCEEDED)
}
