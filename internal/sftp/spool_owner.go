package sftp

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"sshc/internal/filelock"
)

// downloadSpoolOwnerName is the lock file that marks a spool directory as used
// by a running manager.
const downloadSpoolOwnerName = ".owner.lock"

// downloadSpoolQuotaWait bounds the wait for the lock every process takes while
// it counts the spools. The holder only reads the spool directories and writes
// one small file, so a lock still held after this is stuck, not busy.
const downloadSpoolQuotaWait = 5 * time.Second

// lockRelease adapts a filelock release to io.Closer.
type lockRelease func() error

func (release lockRelease) Close() error { return release() }

func holdDownloadSpoolOwner(directory string) (io.Closer, error) {
	release, err := filelock.TryAcquire(filepath.Join(directory, downloadSpoolOwnerName))
	if err != nil {
		return nil, err
	}
	return lockRelease(release), nil
}

// downloadSpoolOwnerState tells whether directory was made by a manager
// (managed) and, if so, whether that manager has gone (inactive).
func downloadSpoolOwnerState(directory string) (managed, inactive bool, resultErr error) {
	ownerPath := filepath.Join(directory, downloadSpoolOwnerName)
	if _, err := os.Lstat(ownerPath); errors.Is(err, os.ErrNotExist) {
		return false, false, nil
	} else if err != nil {
		return true, false, err
	}
	release, err := filelock.TryAcquire(ownerPath)
	if errors.Is(err, filelock.ErrHeld) {
		return true, false, nil
	}
	if err != nil {
		return true, false, err
	}
	return true, true, release()
}

func holdDownloadSpoolQuota(spoolRoot string) (io.Closer, error) {
	release, err := filelock.AcquireWithin(downloadSpoolQuotaPath(spoolRoot), downloadSpoolQuotaWait)
	if err != nil {
		return nil, err
	}
	return lockRelease(release), nil
}
