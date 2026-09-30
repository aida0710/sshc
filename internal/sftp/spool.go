package sftp

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// maxDownloadSpoolBytes caps what every sshc process of the user holds
	// under one spool root at once. A maximum-sized file may be prepared; the
	// reservation rejects more work before another temporary file is created.
	maxDownloadSpoolBytes = int64(512 << 30)
	maxArchiveSpoolBytes  = maxArchiveBytes + int64(maxArchiveEntries*2048) + (1 << 20)
	// downloadSpoolDirectoryPrefix names the directory of one TransferManager
	// under the spool root. Cleanup and the quota count only these.
	downloadSpoolDirectoryPrefix = "sshc-sftp-spool-"
)

var (
	errSpoolRootNotPrivate = errors.New("download spool root is not a private directory of this user")
	// errNoDownloadSpoolRoot is the cause when the caller had no place to
	// offer, for example without a user cache directory.
	errNoDownloadSpoolRoot = errors.New("no download spool root was given")
	errDownloadSpoolClosed = errors.New("the download spool is closed")
)

// downloadSpool is where one TransferManager prepares downloads. The root is
// shared by every sshc process of the user: it holds the quota lock and one
// directory per manager, which records that manager's reservation for the
// others to count. The manager's own directory is made on its first download,
// so a manager that never downloads leaves nothing in the root.
type downloadSpool struct {
	root string
	// rootErr is why the root cannot be used. Downloads then fail with
	// ErrSpoolUnavailable and never fall back to a shared temporary
	// directory, which would bypass the crash cleanup and the quota.
	rootErr error

	mutex         sync.Mutex
	directory     string
	owner         io.Closer
	reservedBytes int64
	closed        bool
}

func newDownloadSpool(spoolRoot string) *downloadSpool {
	spool := &downloadSpool{root: spoolRoot}
	if spoolRoot == "" {
		spool.rootErr = errNoDownloadSpoolRoot
		return spool
	}
	if err := prepareDownloadSpoolRoot(spoolRoot); err != nil {
		spool.rootErr = err
		return spool
	}
	// Spools left by processes that ended without closing go at start, so
	// their plaintext does not wait for the next download.
	cleanupDownloadSpoolDirectories(spoolRoot, time.Now())
	return spool
}

// currentDirectory returns this manager's spool directory and creates it on
// the first call. A failed creation is tried again by the next download.
func (spool *downloadSpool) currentDirectory() (string, error) {
	if spool.rootErr != nil {
		return "", spool.rootErr
	}
	spool.mutex.Lock()
	defer spool.mutex.Unlock()
	if spool.closed {
		return "", errDownloadSpoolClosed
	}
	if spool.directory == "" {
		directory, owner, err := createDownloadSpoolDirectory(spool.root)
		if err != nil {
			return "", err
		}
		spool.directory, spool.owner = directory, owner
	}
	return spool.directory, nil
}

func (spool *downloadSpool) reserve(size int64) error {
	spool.mutex.Lock()
	defer spool.mutex.Unlock()
	reserved, err := reserveDownloadSpool(spool.root, spool.directory, spool.reservedBytes, size, maxDownloadSpoolBytes)
	if err != nil {
		return err
	}
	spool.reservedBytes = reserved
	return nil
}

// release always gives the bytes back in memory. When the file that shows the
// reservation to other processes cannot be rewritten, it keeps the larger
// value until the next reservation or release writes it again.
func (spool *downloadSpool) release(size int64) {
	spool.mutex.Lock()
	defer spool.mutex.Unlock()
	spool.reservedBytes = max(spool.reservedBytes-size, 0)
	if spool.directory == "" {
		return
	}
	quota, err := holdDownloadSpoolQuota(spool.root)
	if err != nil {
		return
	}
	defer quota.Close()
	_ = writeDownloadSpoolReservation(spool.directory, spool.reservedBytes)
}

// close runs after every prepared download of the manager is closed. It
// removes the manager's directory; when that fails, the released owner lock
// lets the next process's cleanup remove it instead.
func (spool *downloadSpool) close() error {
	spool.mutex.Lock()
	defer spool.mutex.Unlock()
	spool.closed = true
	if spool.owner == nil {
		return nil
	}
	err := spool.owner.Close()
	_ = os.RemoveAll(spool.directory)
	spool.owner, spool.directory = nil, ""
	return err
}

// reserveDownloadSpool returns ErrTransferLimit only when the shared quota is
// full, which clears once another download finishes. A spool that cannot be
// used at all is ErrSpoolUnavailable, so callers fail instead of waiting for a
// slot that will never open.
func reserveDownloadSpool(spoolRoot, current string, currentReserved, size, limit int64) (int64, error) {
	if spoolRoot == "" || current == "" {
		return currentReserved, ErrSpoolUnavailable
	}
	if size < 0 || currentReserved < 0 || limit < 0 {
		return currentReserved, ErrTransferLimit
	}
	quota, err := holdDownloadSpoolQuota(spoolRoot)
	if err != nil {
		return currentReserved, spoolUnavailable(err)
	}
	defer quota.Close()
	total, err := activeDownloadSpoolBytes(spoolRoot)
	if err != nil {
		return currentReserved, spoolUnavailable(err)
	}
	if total > limit || size > limit-total {
		return currentReserved, ErrTransferLimit
	}
	next := currentReserved + size
	if next < currentReserved || next > limit {
		return currentReserved, ErrTransferLimit
	}
	if err := writeDownloadSpoolReservation(current, next); err != nil {
		return currentReserved, spoolUnavailable(err)
	}
	return next, nil
}

// spoolSpaceHeadroom stays free after a download is spooled, so a file that
// fits exactly does not leave the filesystem under the engine full.
const spoolSpaceHeadroom = uint64(64 << 20)

// ensureSpoolSpace refuses a download the spool's filesystem cannot hold
// before any of it is written. Downloads reserved at the same moment can
// still run out together; spoolWriteError reports that case the same way.
func ensureSpoolSpace(directory string, size int64) error {
	available, err := spoolFreeBytes(directory)
	if err != nil {
		// Not knowing the free space is no reason to refuse; a full disk is
		// still reported when the write fails.
		return nil
	}
	return checkSpoolSpace(available, size)
}

func checkSpoolSpace(available uint64, size int64) error {
	if size >= 0 && uint64(size) <= available && available-uint64(size) >= spoolSpaceHeadroom {
		return nil
	}
	return fmt.Errorf("%w: %d bytes needed, %d available", ErrSpoolFull, size, available)
}

// spoolWriteError tells a spool that ran out of space apart from a failure on
// the remote side, which the user cannot fix by freeing space on this machine.
func spoolWriteError(err error) error {
	if err != nil && !errors.Is(err, ErrSpoolFull) && isNoSpaceLeft(err) {
		return fmt.Errorf("%w: %w", ErrSpoolFull, err)
	}
	return err
}

// spoolUnavailable reports ErrSpoolUnavailable and keeps the file error that
// made the spool unusable as its cause.
func spoolUnavailable(cause error) error {
	return fmt.Errorf("%w: %w", ErrSpoolUnavailable, cause)
}

type preparedDownloadCache struct {
	download *PreparedDownload
	created  time.Time
}

// preparedSpoolLease keeps a prepared file and its reservation until the
// last clone handed out for it is closed.
type preparedSpoolLease struct {
	spool    *downloadSpool
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
		lease.spool.release(lease.reserved)
	}
}

// createDownloadSpoolDirectory makes one manager's directory under the root.
// MkdirTemp creates it atomically with mode 0700 and a random suffix, so the
// managers of several processes never share one. The owner lock marks it live
// until the manager closes or its process ends.
func createDownloadSpoolDirectory(spoolRoot string) (string, io.Closer, error) {
	directory, err := os.MkdirTemp(spoolRoot, downloadSpoolDirectoryPrefix)
	if err != nil {
		return "", nil, err
	}
	if err := prepareDownloadSpoolDirectory(directory); err != nil {
		_ = os.RemoveAll(directory)
		return "", nil, err
	}
	owner, err := holdDownloadSpoolOwner(directory)
	if err != nil {
		_ = os.RemoveAll(directory)
		return "", nil, err
	}
	return directory, owner, nil
}

// unownedSpoolDirectoryAge is how old a spool directory without an owner lock
// must be before cleanup removes it. Such a directory is either one another
// process is creating right now (between MkdirTemp and holdDownloadSpoolOwner)
// or one left by a process that died there. Only its age tells them apart, and
// a day is far longer than that creation takes.
const unownedSpoolDirectoryAge = 24 * time.Hour

func cleanupDownloadSpoolDirectories(spoolRoot string, now time.Time) {
	entries, err := os.ReadDir(spoolRoot)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), downloadSpoolDirectoryPrefix) {
			continue
		}
		candidate := filepath.Join(spoolRoot, entry.Name())
		info, err := os.Lstat(candidate)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || !downloadSpoolDirectoryTrusted(info, candidate) {
			continue
		}
		managed, inactive, lockErr := downloadSpoolOwnerState(candidate)
		if lockErr != nil || managed && !inactive || !managed && now.Sub(info.ModTime()) < unownedSpoolDirectoryAge {
			continue
		}
		// RemoveAll does not follow symlinks found within the directory, and
		// the root is the user's own (prepareDownloadSpoolRoot), so no other
		// account can replace this path between Lstat and removal.
		_ = os.RemoveAll(candidate)
	}
}

func activeDownloadSpoolBytes(spoolRoot string) (int64, error) {
	if spoolRoot == "" {
		return 0, nil
	}
	entries, err := os.ReadDir(spoolRoot)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, entry := range entries {
		candidate := filepath.Join(spoolRoot, entry.Name())
		if !strings.HasPrefix(entry.Name(), downloadSpoolDirectoryPrefix) {
			continue
		}
		info, err := os.Lstat(candidate)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || !downloadSpoolDirectoryTrusted(info, candidate) {
			continue
		}
		actual := downloadSpoolTreeBytes(candidate)
		reserved, reservationErr := readDownloadSpoolReservation(candidate)
		// An empty file is a reservation whose first write failed, for example
		// on a full disk. It records nothing beyond the files themselves.
		if errors.Is(reservationErr, os.ErrNotExist) || errors.Is(reservationErr, io.EOF) {
			reserved = actual
		} else if reservationErr != nil {
			return 0, fmt.Errorf("read the reservation of %s: %w", entry.Name(), reservationErr)
		} else if reserved < 0 {
			return 0, fmt.Errorf("the reservation of %s is negative", entry.Name())
		}
		if actual > reserved {
			reserved = actual
		}
		if reserved > maxDownloadSpoolBytes-total {
			return maxDownloadSpoolBytes + 1, nil
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
