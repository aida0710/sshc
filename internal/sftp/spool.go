package sftp

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// A maximum-sized file may be prepared at once. The process-wide reservation
	// rejects additional work before another temporary file is created.
	maxProcessDownloadSpoolBytes = int64(512 << 30)
	maxArchiveSpoolBytes         = maxArchiveBytes + int64(maxArchiveEntries*2048) + (1 << 20)
)

var (
	processSpoolOnce  sync.Once
	processSpoolDir   string
	processSpoolRoot  string
	processSpoolOwner []io.Closer
	processSpoolMu    sync.Mutex
	processSpoolBytes int64
)

func reserveProcessSpool(size int64) error {
	processSpoolMu.Lock()
	defer processSpoolMu.Unlock()
	reserved, err := reserveDownloadSpool(
		processSpoolRoot, processSpoolDir, processSpoolBytes, size, maxProcessDownloadSpoolBytes,
	)
	if err != nil {
		return err
	}
	processSpoolBytes = reserved
	return nil
}

func releaseProcessSpool(size int64) {
	processSpoolMu.Lock()
	defer processSpoolMu.Unlock()
	remaining := processSpoolBytes - size
	if remaining < 0 {
		remaining = 0
	}
	quota, err := holdDownloadSpoolQuota(processSpoolRoot)
	if err != nil {
		return
	}
	defer quota.Close()
	if err := writeDownloadSpoolReservation(processSpoolDir, remaining); err != nil {
		return
	}
	processSpoolBytes = remaining
}

func reserveDownloadSpool(temporaryRoot, current string, currentReserved, size, limit int64) (int64, error) {
	if temporaryRoot == "" || current == "" || size < 0 || currentReserved < 0 || limit < 0 {
		return currentReserved, ErrTransferLimit
	}
	quota, err := holdDownloadSpoolQuota(temporaryRoot)
	if err != nil {
		return currentReserved, ErrTransferLimit
	}
	defer quota.Close()
	total, err := activeDownloadSpoolBytes(temporaryRoot)
	if err != nil || total > limit || size > limit-total {
		return currentReserved, ErrTransferLimit
	}
	next := currentReserved + size
	if next < currentReserved || next > limit {
		return currentReserved, ErrTransferLimit
	}
	if err := writeDownloadSpoolReservation(current, next); err != nil {
		return currentReserved, ErrTransferLimit
	}
	return next, nil
}

type preparedDownloadCache struct {
	download *PreparedDownload
	created  time.Time
}

type preparedSpoolLease struct {
	mutex    sync.Mutex
	path     string
	reserved int64
	refs     int
}

func (lease *preparedSpoolLease) acquire() {
	lease.mutex.Lock()
	lease.refs++
	lease.mutex.Unlock()
}

func (lease *preparedSpoolLease) release() {
	lease.mutex.Lock()
	lease.refs--
	last := lease.refs == 0
	lease.mutex.Unlock()
	if last {
		_ = os.Remove(lease.path)
		releaseProcessSpool(lease.reserved)
	}
}

func downloadSpoolDirectory() string {
	processSpoolOnce.Do(func() {
		processSpoolRoot = os.TempDir()
		processSpoolDir = createDownloadSpoolDirectory(os.TempDir())
	})
	return processSpoolDir
}

func createDownloadSpoolDirectory(temporaryRoot string) string {
	cleanupDownloadSpoolDirectories(temporaryRoot, time.Now())
	// A predictable name in a shared temporary directory permits a local attacker
	// to race startup with a symlink and observe the plaintext spool. MkdirTemp
	// creates the final directory atomically with mode 0700 and a random suffix.
	root, err := os.MkdirTemp(temporaryRoot, "sshc-sftp-spool-")
	if err != nil {
		return ""
	}
	if err := prepareDownloadSpoolDirectory(root); err != nil {
		_ = os.RemoveAll(root)
		return ""
	}
	owner, err := holdDownloadSpoolOwner(root)
	if err != nil {
		_ = os.RemoveAll(root)
		return ""
	}
	// Keep every owner handle alive for the process lifetime. Production creates
	// one directory; retaining all handles also makes repeated isolated manager
	// construction safe in tests and embedding scenarios.
	processSpoolOwner = append(processSpoolOwner, owner)
	return root
}

func cleanupDownloadSpoolDirectories(temporaryRoot string, now time.Time) {
	const staleAfter = 24 * time.Hour
	entries, err := os.ReadDir(temporaryRoot)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "sshc-sftp-spool-") {
			continue
		}
		candidate := filepath.Join(temporaryRoot, entry.Name())
		info, err := os.Lstat(candidate)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || !downloadSpoolDirectoryTrusted(info, candidate) {
			continue
		}
		managed, inactive, lockErr := downloadSpoolOwnerState(candidate)
		if lockErr != nil || managed && !inactive || !managed && now.Sub(info.ModTime()) < staleAfter {
			continue
		}
		// RemoveAll does not follow symlinks found within the directory. The
		// parent temp directory's sticky bit plus the ownership check prevents a
		// different local UID from replacing this path between Lstat and removal.
		_ = os.RemoveAll(candidate)
	}
}

func activeDownloadSpoolBytes(temporaryRoot string) (int64, error) {
	if temporaryRoot == "" {
		return 0, nil
	}
	entries, err := os.ReadDir(temporaryRoot)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, entry := range entries {
		candidate := filepath.Join(temporaryRoot, entry.Name())
		if !strings.HasPrefix(entry.Name(), "sshc-sftp-spool-") {
			continue
		}
		info, err := os.Lstat(candidate)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || !downloadSpoolDirectoryTrusted(info, candidate) {
			continue
		}
		actual := downloadSpoolTreeBytes(candidate)
		reserved, reservationErr := readDownloadSpoolReservation(candidate)
		if errors.Is(reservationErr, os.ErrNotExist) {
			reserved = actual
		} else if reservationErr != nil || reserved < 0 {
			return 0, ErrTransferLimit
		}
		if actual > reserved {
			reserved = actual
		}
		if reserved > maxProcessDownloadSpoolBytes-total {
			return maxProcessDownloadSpoolBytes + 1, nil
		}
		total += reserved
	}
	return total, nil
}

func downloadSpoolTreeBytes(root string) int64 {
	var total int64
	_ = filepath.WalkDir(root, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if info, infoErr := entry.Info(); infoErr == nil && info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total
}
