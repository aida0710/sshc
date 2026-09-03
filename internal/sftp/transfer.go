package sftp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const AbsentRevision = "absent"

var transferIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`)
var sourceFingerprintPattern = regexp.MustCompile(`^tree-sha256:[0-9a-f]{64}$`)
var uploadPartNamePattern = regexp.MustCompile(`^\..+\.sshc-upload-[A-Za-z0-9_-]{8,128}\.part$`)
var editorTemporaryNamePattern = regexp.MustCompile(`^\..+\.sshc-[0-9a-f]{24}\.tmp$`)

// TransferManager serializes requests that operate on the same resumable part file.
// Different files remain independent so a large queue can make bounded parallel progress.
type TransferManager struct {
	Service   *Service
	mutex     sync.Mutex
	locks     map[string]*transferLock
	remotes   map[string]Remote
	downloads map[string]preparedDownloadCache
	spoolDir  string
	closed    bool
	closeOnce sync.Once
	closeErr  error

	jobsMutex sync.Mutex
	jobs      map[string]*transferJobRecord
	jobOrder  []string
	// clearCompletedAfter が 0 なら、完了項目は手動でだけ消える。
	clearCompletedAfter time.Duration
	// processingStopped の間は start を通さない。実行中のものは走り切る。
	processingStopped    bool
	activeJobs           int
	maxConcurrent        int
	largeFileThreshold   int64
	largeFileParallelism int
	largeFileChunkBytes  int64
	now                  func() time.Time
	dataPlane            map[string]int
	remoteJobsMutex      sync.Mutex
	remoteCancels        map[string]context.CancelFunc
	queuePath            string
	lastQueuePersist     time.Time
	queuePersistError    error
}

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

type transferLock struct {
	mutex sync.RWMutex
	refs  int
}

func NewTransferManager(service *Service) *TransferManager {
	manager := &TransferManager{Service: service, locks: make(map[string]*transferLock), remotes: make(map[string]Remote), downloads: make(map[string]preparedDownloadCache), spoolDir: downloadSpoolDirectory(), remoteCancels: make(map[string]context.CancelFunc)}
	manager.ConfigureJobs(DefaultTransferConcurrency, time.Now)
	return manager
}

// Close releases resources intentionally retained between transfer requests.
// The HTTP server calls it only after its admission barrier has drained every
// request, so no data-plane operation can still be using these handles.
func (m *TransferManager) Close() error {
	if m == nil {
		return nil
	}
	m.closeOnce.Do(func() {
		m.remoteJobsMutex.Lock()
		for id, cancel := range m.remoteCancels {
			cancel()
			delete(m.remoteCancels, id)
		}
		m.remoteJobsMutex.Unlock()
		m.mutex.Lock()
		m.closed = true
		remotes := make([]Remote, 0, len(m.remotes))
		for key, remote := range m.remotes {
			delete(m.remotes, key)
			remotes = append(remotes, remote)
		}
		downloads := make([]*PreparedDownload, 0, len(m.downloads))
		for id, cached := range m.downloads {
			delete(m.downloads, id)
			downloads = append(downloads, cached.download)
		}
		m.mutex.Unlock()
		var joined []error
		for _, remote := range remotes {
			if err := remote.Close(); err != nil {
				joined = append(joined, err)
			}
		}
		for _, download := range downloads {
			if err := download.Close(); err != nil {
				joined = append(joined, err)
			}
		}
		m.closeErr = errors.Join(joined...)
	})
	return m.closeErr
}

func (m *TransferManager) isClosed() bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.closed
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

// PrepareOwnedDownload caches the immutable local spool by job so a network
// retry reuses the exact same representation instead of downloading and
// hashing the remote file again. Callers receive independent file handles.
func (m *TransferManager) PrepareOwnedDownload(ctx context.Context, id, alias, remotePath string) (*PreparedDownload, error) {
	return m.prepareOwnedSpool(id, func() (*PreparedDownload, int64, error) {
		var reserved int64
		m.resetDownloadParts(id)
		threshold, parallelism, chunkBytes, err := m.transferSplitSettings(id)
		if err != nil {
			return nil, 0, err
		}
		prepared, err := m.Service.prepareDownload(ctx, alias, remotePath, m.spoolDir, func(size int64) error {
			if err := reserveProcessSpool(size); err != nil {
				return err
			}
			reserved = size
			return nil
		}, threshold, parallelism, chunkBytes, func(part DownloadPartProgress) {
			m.recordDownloadPart(id, part)
		})
		if err != nil {
			if reserved > 0 {
				releaseProcessSpool(reserved)
			}
			return nil, 0, err
		}
		return prepared, reserved, nil
	})
}

func (m *TransferManager) resetDownloadParts(id string) {
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	if record := m.jobs[id]; record != nil {
		record.job.DownloadParts = nil
	}
}

func (m *TransferManager) recordDownloadPart(id string, part DownloadPartProgress) {
	if part.Index < 0 || part.Index >= MaxLargeFileParallelism || part.TotalBytes < 0 ||
		part.TransferredBytes < 0 || part.TransferredBytes > part.TotalBytes {
		return
	}
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	record := m.jobs[id]
	if record == nil || record.job.Direction != TransferDownload || record.job.Status != TransferRunning {
		return
	}
	for len(record.job.DownloadParts) <= part.Index {
		record.job.DownloadParts = append(record.job.DownloadParts, DownloadPartProgress{Index: len(record.job.DownloadParts)})
	}
	record.job.DownloadParts[part.Index] = part
}

func (m *TransferManager) transferSplitSettings(id string) (int64, int, int64, error) {
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	record := m.jobs[id]
	if record == nil {
		return 0, 0, 0, ErrTransferNotFound
	}
	threshold, parallelism, chunkBytes := m.largeFileThreshold, m.largeFileParallelism, m.largeFileChunkBytes
	if record.job.LargeFileThresholdBytes != 0 {
		threshold = record.job.LargeFileThresholdBytes
	}
	if record.job.LargeFileParallelism != 0 {
		parallelism = record.job.LargeFileParallelism
	}
	if record.job.LargeFileChunkBytes != 0 {
		chunkBytes = record.job.LargeFileChunkBytes
	}
	return threshold, parallelism, chunkBytes, nil
}

func (m *TransferManager) PrepareOwnedArchive(ctx context.Context, id, alias, remotePath string) (*PreparedDownload, error) {
	return m.prepareOwnedSpool(id, func() (*PreparedDownload, int64, error) {
		if err := reserveProcessSpool(maxArchiveSpoolBytes); err != nil {
			return nil, 0, err
		}
		prepared, err := m.Service.prepareArchive(ctx, alias, remotePath, m.spoolDir, maxArchiveSpoolBytes)
		if err != nil {
			releaseProcessSpool(maxArchiveSpoolBytes)
			return nil, 0, err
		}
		return prepared, maxArchiveSpoolBytes, nil
	})
}

func (m *TransferManager) prepareOwnedSpool(id string, build func() (*PreparedDownload, int64, error)) (*PreparedDownload, error) {
	if m.isClosed() {
		return nil, ErrUnavailable
	}
	if m.spoolDir == "" {
		// Never fall back to the system temp directory when the private spool
		// could not be initialized; that would bypass crash cleanup and quota.
		return nil, ErrTransferLimit
	}
	m.sweepPreparedDownloads()
	unlock := m.lock("", "\x00download-prepare:"+id)
	defer unlock()
	m.mutex.Lock()
	if existing, exists := m.downloads[id]; exists {
		clone, err := clonePreparedDownload(existing.download)
		m.mutex.Unlock()
		return clone, err
	}
	m.mutex.Unlock()
	prepared, reserved, err := build()
	if err != nil {
		return nil, err
	}
	if !m.canInstallPrepared(id) {
		_ = prepared.Close()
		releaseProcessSpool(reserved)
		return nil, ErrTransferState
	}
	m.mutex.Lock()
	if m.closed {
		m.mutex.Unlock()
		_ = prepared.Close()
		releaseProcessSpool(reserved)
		return nil, ErrUnavailable
	}
	lease := &preparedSpoolLease{path: prepared.name, reserved: reserved, refs: 1}
	prepared.remove, prepared.lease = false, lease
	m.downloads[id] = preparedDownloadCache{download: prepared, created: time.Now()}
	clone, cloneErr := clonePreparedDownload(prepared)
	m.mutex.Unlock()
	return clone, cloneErr
}

func clonePreparedDownload(source *PreparedDownload) (*PreparedDownload, error) {
	file, err := os.Open(source.name)
	if err != nil {
		return nil, err
	}
	if source.lease == nil {
		_ = file.Close()
		return nil, ErrTransferState
	}
	source.lease.acquire()
	return &PreparedDownload{file: file, name: source.name, lease: source.lease, Size: source.Size, Revision: source.Revision}, nil
}

func (m *TransferManager) canInstallPrepared(id string) bool {
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	record := m.jobs[id]
	return record != nil && record.job.Direction == TransferDownload && record.job.Status == TransferRunning
}

func (m *TransferManager) releasePreparedDownload(id string) {
	m.mutex.Lock()
	cached, ok := m.downloads[id]
	if ok {
		delete(m.downloads, id)
	}
	m.mutex.Unlock()
	if ok {
		_ = cached.download.Close()
	}
}

func (m *TransferManager) sweepPreparedDownloads() {
	const cacheTTL = time.Hour
	now := time.Now()
	var expired []*PreparedDownload
	m.mutex.Lock()
	for id, cached := range m.downloads {
		if now.Sub(cached.created) <= cacheTTL {
			continue
		}
		delete(m.downloads, id)
		expired = append(expired, cached.download)
	}
	m.mutex.Unlock()
	for _, download := range expired {
		_ = download.Close()
	}
}

// StartOwned chooses sequential or ranged upload from the settings captured by
// the job. The owner lock serializes preparation with append, complete, pause
// and cancel; ranged writes later share this lock with each other.
func (m *TransferManager) StartOwned(ctx context.Context, alias, id, remotePath string, options StartUploadOptions) (ResumableUpload, error) {
	unlock := m.lock("", "\x00job-owner:"+id)
	defer unlock()
	done, err := m.KeepJobActive(id)
	if err != nil {
		return ResumableUpload{}, err
	}
	defer done()
	if err := m.AuthorizeUpload(id, alias, remotePath, options.Size, false); err != nil {
		return ResumableUpload{}, err
	}
	if options.SourceFingerprint != "" && !sourceFingerprintPattern.MatchString(options.SourceFingerprint) {
		return ResumableUpload{}, ErrInvalidTransfer
	}
	m.jobsMutex.Lock()
	record := m.jobs[id]
	if record == nil {
		m.jobsMutex.Unlock()
		return ResumableUpload{}, ErrTransferNotFound
	}
	if options.SourceFingerprint != "" && record.job.SourceFingerprint != "" && record.job.SourceFingerprint != options.SourceFingerprint {
		m.jobsMutex.Unlock()
		return ResumableUpload{}, ErrConflict
	}
	if options.SourceFingerprint != "" && record.job.SourceFingerprint == "" {
		record.job.SourceFingerprint = options.SourceFingerprint
	}
	job := record.job
	options.Overwrite = job.Overwrite
	options.ExpectedRevision = job.ExpectedRevision
	threshold, parallelism, chunkBytes := m.largeFileThreshold, m.largeFileParallelism, m.largeFileChunkBytes
	if job.LargeFileThresholdBytes != 0 {
		threshold = job.LargeFileThresholdBytes
	}
	if job.LargeFileParallelism != 0 {
		parallelism = job.LargeFileParallelism
	}
	if job.LargeFileChunkBytes != 0 {
		chunkBytes = job.LargeFileChunkBytes
	}
	completed := append([]UploadRange(nil), job.UploadRanges...)
	m.jobsMutex.Unlock()
	parallel := options.Size >= threshold && parallelism > 1 && chunkBytes > 0
	var upload ResumableUpload
	if parallel {
		upload, err = m.startParallelUpload(ctx, alias, id, remotePath, options, completed, parallelism, chunkBytes)
	} else {
		upload, err = m.Start(ctx, alias, id, remotePath, options)
		upload.Parallelism, upload.ChunkBytes = 1, chunkBytes
	}
	if err != nil {
		return ResumableUpload{}, err
	}
	total, offset := options.Size, upload.Offset
	m.jobsMutex.Lock()
	if record := m.jobs[id]; record != nil {
		record.job.ExpectedRevision = upload.ExpectedRevision
		record.job.UploadRanges = append([]UploadRange(nil), upload.CompletedRanges...)
	}
	m.jobsMutex.Unlock()
	if _, err := m.updateUploadJob(id, UpdateTransferJob{
		Action: TransferProgressAction, TransferredBytes: &offset, TotalBytes: &total, ResetProgress: true,
	}); err != nil {
		m.releaseRemote(alias, id, upload.Path)
		return ResumableUpload{}, err
	}
	return upload, nil
}

// AppendRangeOwned streams one complete configured range to an independent
// SFTP connection. Successful ranges are persisted and coalesced, so an
// interrupted browser or CLI only resends ranges whose acknowledgement was
// not made durable.
func (m *TransferManager) AppendRangeOwned(ctx context.Context, alias, id, remotePath string, offset, total, length int64, contents io.Reader) (ResumableUpload, error) {
	unlock := m.readLock("", "\x00job-owner:"+id)
	defer unlock()
	done, err := m.KeepJobActive(id)
	if err != nil {
		return ResumableUpload{}, err
	}
	defer done()
	if err := m.AuthorizeUpload(id, alias, remotePath, total, false); err != nil {
		return ResumableUpload{}, err
	}
	m.jobsMutex.Lock()
	record := m.jobs[id]
	if record == nil {
		m.jobsMutex.Unlock()
		return ResumableUpload{}, ErrTransferNotFound
	}
	threshold, parallelism, chunkBytes := m.largeFileThreshold, m.largeFileParallelism, m.largeFileChunkBytes
	if record.job.LargeFileThresholdBytes != 0 {
		threshold = record.job.LargeFileThresholdBytes
	}
	if record.job.LargeFileParallelism != 0 {
		parallelism = record.job.LargeFileParallelism
	}
	if record.job.LargeFileChunkBytes != 0 {
		chunkBytes = record.job.LargeFileChunkBytes
	}
	expectedRevision := record.job.ExpectedRevision
	already := uploadRangeCovered(record.job.UploadRanges, offset, length)
	m.jobsMutex.Unlock()
	if total < threshold || parallelism <= 1 || chunkBytes <= 0 || offset < 0 || length <= 0 || offset%chunkBytes != 0 ||
		length != min(chunkBytes, total-offset) || offset+length > total {
		return ResumableUpload{}, ErrInvalidTransfer
	}
	if !already {
		if err := m.writeUploadRange(ctx, alias, id, remotePath, offset, total, length, contents); err != nil {
			return ResumableUpload{}, err
		}
	} else if err := consumeExactUploadRange(ctx, io.Discard, contents, length); err != nil {
		return ResumableUpload{}, err
	}
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	record = m.jobs[id]
	if record == nil || record.job.Status != TransferRunning || record.job.ExpectedRevision != expectedRevision {
		return ResumableUpload{}, ErrTransferState
	}
	before := append([]UploadRange(nil), record.job.UploadRanges...)
	record.job.UploadRanges = addUploadRange(record.job.UploadRanges, UploadRange{Offset: offset, Size: length})
	record.job.TransferredBytes = uploadRangeBytes(record.job.UploadRanges)
	now := m.now().UTC()
	m.updateRateLocked(record, now)
	record.job.UpdatedAt = now
	if err := m.persistJobsLocked(true); err != nil {
		record.job.UploadRanges = before
		record.job.TransferredBytes = uploadRangeBytes(before)
		return ResumableUpload{}, err
	}
	return ResumableUpload{ID: id, Path: path.Clean(remotePath), Offset: record.job.TransferredBytes, Size: total,
		ExpectedRevision: expectedRevision, CompletedRanges: append([]UploadRange(nil), record.job.UploadRanges...),
		Parallelism: parallelism, ChunkBytes: chunkBytes}, nil
}

func (m *TransferManager) startParallelUpload(
	ctx context.Context, alias, id, remotePath string, options StartUploadOptions, completed []UploadRange, parallelism int, chunkBytes int64,
) (_ ResumableUpload, resultErr error) {
	cleaned, err := resumablePath(id, remotePath)
	if err != nil || options.Size < 0 || validateUploadRanges(completed, options.Size) != nil {
		return ResumableUpload{}, ErrInvalidTransfer
	}
	if options.Size > maxRegularFileTransferBytes {
		return ResumableUpload{}, ErrTransferTooLarge
	}
	unlock := m.lock(alias, cleaned)
	defer unlock()
	remote, err := m.Service.openRequest(ctx, alias)
	if err != nil {
		return ResumableUpload{}, err
	}
	defer remote.Close()
	expected, err := expectedTargetRevision(ctx, remote, cleaned, options)
	if err != nil {
		return ResumableUpload{}, err
	}
	part := uploadPartPath(cleaned, id)
	info, err := remote.Lstat(part)
	reset := false
	if errors.Is(err, fs.ErrNotExist) {
		if err := m.clearParallelUploadRangesBeforeReset(id, completed); err != nil {
			return ResumableUpload{}, err
		}
		completed = nil
		created, createErr := remote.Create(part)
		if createErr != nil {
			return ResumableUpload{}, createErr
		}
		if closeErr := created.Close(); closeErr != nil {
			return ResumableUpload{}, closeErr
		}
		reset = true
	} else if err != nil {
		return ResumableUpload{}, err
	} else if !info.Mode().IsRegular() {
		return ResumableUpload{}, ErrNotRegularFile
	} else if info.Size() != options.Size {
		if err := m.clearParallelUploadRangesBeforeReset(id, completed); err != nil {
			return ResumableUpload{}, err
		}
		completed = nil
		reset = true
	}
	file, err := remote.OpenFile(part, os.O_WRONLY)
	if err != nil {
		return ResumableUpload{}, err
	}
	if reset {
		err = file.Truncate(options.Size)
	}
	closeErr := file.Close()
	if err != nil {
		return ResumableUpload{}, err
	}
	if closeErr != nil {
		return ResumableUpload{}, closeErr
	}
	if reset {
		completed = nil
	}
	return ResumableUpload{ID: id, Path: cleaned, Offset: uploadRangeBytes(completed), Size: options.Size,
		ExpectedRevision: expected, CompletedRanges: append([]UploadRange(nil), completed...),
		Parallelism: parallelism, ChunkBytes: chunkBytes}, nil
}

// clearParallelUploadRangesBeforeReset commits the loss of resumable ranges
// before the remote part is created or truncated. If the process stops between
// these two operations, the next start safely resends every range instead of
// trusting offsets that belonged to the previous part file.
func (m *TransferManager) clearParallelUploadRangesBeforeReset(id string, completed []UploadRange) error {
	if len(completed) == 0 {
		return nil
	}
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	record := m.jobs[id]
	if record == nil || record.job.Status != TransferRunning {
		return ErrTransferState
	}
	original := *record
	record.job.UploadRanges = nil
	record.job.TransferredBytes = 0
	record.job.BytesPerSecond = 0
	record.job.RemainingSeconds = -1
	record.sampleBytes = 0
	record.sampleAt = m.now().UTC()
	record.job.UpdatedAt = record.sampleAt
	if err := m.persistJobsLocked(true); err != nil {
		*record = original
		return err
	}
	return nil
}

func (m *TransferManager) writeUploadRange(ctx context.Context, alias, id, remotePath string, offset, total, length int64, contents io.Reader) error {
	cleaned, err := resumablePath(id, remotePath)
	if err != nil {
		return err
	}
	unlock := m.readLock(alias, cleaned)
	defer unlock()
	remote, err := m.Service.openRequest(ctx, alias)
	if err != nil {
		return err
	}
	defer remote.Close()
	part := uploadPartPath(cleaned, id)
	info, err := remote.Lstat(part)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() != total {
		return ErrOffsetMismatch
	}
	file, err := remote.OpenFile(part, os.O_WRONLY)
	if err != nil {
		return err
	}
	if _, err = file.Seek(offset, io.SeekStart); err == nil {
		err = consumeExactUploadRange(ctx, file, contents, length)
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func consumeExactUploadRange(ctx context.Context, destination io.Writer, contents io.Reader, length int64) error {
	written, err := copyContext(ctx, destination, io.LimitReader(contents, length), 0)
	if err != nil {
		return err
	}
	if written != length {
		return io.ErrUnexpectedEOF
	}
	var extra [1]byte
	count, readErr := contents.Read(extra[:])
	if count != 0 {
		return ErrTransferTooLarge
	}
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return readErr
	}
	return nil
}

func validateUploadRanges(ranges []UploadRange, total int64) error {
	previousEnd := int64(0)
	for index, portion := range ranges {
		if portion.Offset < 0 || portion.Size <= 0 || portion.Offset > total || portion.Size > total-portion.Offset ||
			(index > 0 && portion.Offset <= previousEnd) {
			return ErrInvalidTransfer
		}
		previousEnd = portion.Offset + portion.Size
	}
	return nil
}

func uploadRangeCovered(ranges []UploadRange, offset, size int64) bool {
	for _, portion := range ranges {
		if offset >= portion.Offset && offset+size <= portion.Offset+portion.Size {
			return true
		}
	}
	return false
}

func addUploadRange(ranges []UploadRange, added UploadRange) []UploadRange {
	all := append(append([]UploadRange(nil), ranges...), added)
	sort.Slice(all, func(i, j int) bool { return all[i].Offset < all[j].Offset })
	merged := make([]UploadRange, 0, len(all))
	for _, portion := range all {
		if len(merged) == 0 || merged[len(merged)-1].Offset+merged[len(merged)-1].Size < portion.Offset {
			merged = append(merged, portion)
			continue
		}
		end := max(merged[len(merged)-1].Offset+merged[len(merged)-1].Size, portion.Offset+portion.Size)
		merged[len(merged)-1].Size = end - merged[len(merged)-1].Offset
	}
	return merged
}

func uploadRangeBytes(ranges []UploadRange) int64 {
	var total int64
	for _, portion := range ranges {
		total += portion.Size
	}
	return total
}

// AppendOwned advances both the remote part and its server-side job in one
// serialized request. A browser disconnect after the write cannot leave the
// job waiting for a second client-authored progress request.
func (m *TransferManager) AppendOwned(ctx context.Context, alias, id, remotePath string, offset, total int64, contents []byte) (ResumableUpload, error) {
	unlock := m.lock("", "\x00job-owner:"+id)
	defer unlock()
	done, err := m.KeepJobActive(id)
	if err != nil {
		return ResumableUpload{}, err
	}
	defer done()
	if err := m.AuthorizeUpload(id, alias, remotePath, total, false); err != nil {
		return ResumableUpload{}, err
	}
	if acknowledged, ok, err := m.replayAcknowledgedAppend(id, alias, remotePath, offset, total, len(contents)); err != nil {
		return ResumableUpload{}, err
	} else if ok {
		return acknowledged, nil
	}
	upload, err := m.Append(ctx, alias, id, remotePath, offset, total, contents)
	if err != nil {
		return ResumableUpload{}, err
	}
	if _, err := m.updateUploadJob(id, UpdateTransferJob{Action: TransferProgressAction, TransferredBytes: &upload.Offset}); err != nil {
		m.releaseRemote(alias, id, upload.Path)
		return ResumableUpload{}, err
	}
	m.jobsMutex.Lock()
	if record := m.jobs[id]; record != nil {
		upload.ExpectedRevision = record.job.ExpectedRevision
		upload.ChunkBytes = m.largeFileChunkBytes
		if record.job.LargeFileChunkBytes != 0 {
			upload.ChunkBytes = record.job.LargeFileChunkBytes
		}
	}
	m.jobsMutex.Unlock()
	upload.Parallelism = 1
	upload.CompletedRanges = []UploadRange{}
	return upload, nil
}

// CompleteOwned publishes the part and commits the completed job before the
// request returns. This removes the former rename-then-client-update window.
func (m *TransferManager) CompleteOwned(ctx context.Context, alias, id, remotePath string, total int64, expectedRevision, sourceFingerprint string) (Transfer, error) {
	unlock := m.lock("", "\x00job-owner:"+id)
	defer unlock()
	if replay, ok, err := m.replayCompletedUpload(id, alias, remotePath, total); err != nil {
		return Transfer{}, err
	} else if ok {
		return replay, nil
	}
	done, err := m.KeepJobActive(id)
	if err != nil {
		return Transfer{}, err
	}
	defer done()
	if err := m.AuthorizeUpload(id, alias, remotePath, total, false); err != nil {
		return Transfer{}, err
	}
	m.jobsMutex.Lock()
	record := m.jobs[id]
	if record == nil || record.job.ExpectedRevision == "" || sourceFingerprint == "" ||
		record.job.ExpectedRevision != expectedRevision ||
		(record.job.SourceFingerprint != "" && record.job.SourceFingerprint != sourceFingerprint) {
		m.jobsMutex.Unlock()
		return Transfer{}, ErrConflict
	}
	if record.job.SourceFingerprint == "" {
		record.job.SourceFingerprint = sourceFingerprint
	}
	threshold, parallelism := m.largeFileThreshold, m.largeFileParallelism
	if record.job.LargeFileThresholdBytes != 0 {
		threshold = record.job.LargeFileThresholdBytes
	}
	if record.job.LargeFileParallelism != 0 {
		parallelism = record.job.LargeFileParallelism
	}
	if total >= threshold && parallelism > 1 && uploadRangeBytes(record.job.UploadRanges) != total {
		m.jobsMutex.Unlock()
		return Transfer{}, ErrUploadIncomplete
	}
	m.jobsMutex.Unlock()
	transfer, err := m.Complete(ctx, alias, id, remotePath, total, expectedRevision, sourceFingerprint)
	if err != nil {
		return Transfer{}, err
	}
	if _, err := m.updateUploadJob(id, UpdateTransferJob{Action: TransferProgressAction, TransferredBytes: &total}); err != nil {
		return Transfer{}, err
	}
	if _, err := m.updateUploadJob(id, UpdateTransferJob{Action: TransferCompleteAction, TransferredBytes: &total}); err != nil {
		return Transfer{}, err
	}
	return transfer, nil
}

func (m *TransferManager) replayAcknowledgedAppend(id, alias, remotePath string, offset, total int64, chunkSize int) (ResumableUpload, bool, error) {
	cleaned, err := cleanPublicPath(remotePath, false)
	if err != nil {
		return ResumableUpload{}, false, err
	}
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	record := m.jobs[id]
	if record == nil || record.job.Direction != TransferUpload || record.job.Alias != alias || record.job.RemotePath != cleaned || record.job.TotalBytes != total {
		return ResumableUpload{}, false, ErrConflict
	}
	end := offset + int64(chunkSize)
	if record.job.TransferredBytes == offset {
		return ResumableUpload{}, false, nil
	}
	if record.job.TransferredBytes != end {
		return ResumableUpload{}, false, ErrOffsetMismatch
	}
	chunkBytes := m.largeFileChunkBytes
	if record.job.LargeFileChunkBytes != 0 {
		chunkBytes = record.job.LargeFileChunkBytes
	}
	return ResumableUpload{
		ID: id, Path: cleaned, Offset: end, Size: total, ExpectedRevision: record.job.ExpectedRevision,
		CompletedRanges: []UploadRange{}, Parallelism: 1, ChunkBytes: chunkBytes,
	}, true, nil
}

func (m *TransferManager) replayCompletedUpload(id, alias, remotePath string, total int64) (Transfer, bool, error) {
	cleaned, err := cleanPublicPath(remotePath, false)
	if err != nil {
		return Transfer{}, false, err
	}
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	record := m.jobs[id]
	if record == nil {
		return Transfer{}, false, ErrTransferNotFound
	}
	job := record.job
	if job.Direction != TransferUpload || job.Kind != TransferFile || job.Alias != alias || job.RemotePath != cleaned || job.TotalBytes != total {
		return Transfer{}, false, ErrConflict
	}
	if job.Status != TransferCompleted {
		return Transfer{}, false, nil
	}
	return Transfer{Path: cleaned, Bytes: total}, true, nil
}

// CancelOwned first closes the queue slot to further writes, then removes the
// unpublished part. A cleanup failure remains visible to the caller, while the
// cancelled state still prevents a racing append from writing more bytes.
func (m *TransferManager) CancelOwned(ctx context.Context, alias, id, remotePath string) error {
	unlock := m.lock("", "\x00job-owner:"+id)
	defer unlock()
	m.jobsMutex.Lock()
	m.initializeJobsLocked()
	record := m.jobs[id]
	if record != nil && terminalTransferStatus(record.job.Status) {
		m.jobsMutex.Unlock()
		return nil
	}
	// A job without a recorded remote revision, acknowledged byte, or completed
	// range has not successfully prepared its deterministic remote part. This
	// includes a connection failure during the first preparation attempt, even
	// though the source fingerprint was already recorded for a safe retry.
	pristine := record != nil && !uploadJobHasRemotePart(record.job)
	m.jobsMutex.Unlock()
	if pristine {
		_, err := m.updateUploadJob(id, UpdateTransferJob{Action: TransferCancelAction})
		return err
	}
	done, err := m.KeepJobActive(id)
	if errors.Is(err, ErrTransferNotFound) {
		// Queue state is intentionally process-local. After an engine restart the
		// deterministic unpublished part can outlive its job record, so DELETE must
		// still remove that exact part. Cancel validates both the transfer ID and
		// remote path and never addresses the published target.
		return m.Cancel(ctx, alias, id, remotePath)
	}
	if err != nil {
		return err
	}
	defer done()
	if err := m.AuthorizeUpload(id, alias, remotePath, -1, true); err != nil {
		return err
	}
	if err := m.Cancel(ctx, alias, id, remotePath); err != nil {
		_, _ = m.updateUploadJob(id, UpdateTransferJob{Action: TransferFailAction, Problem: "sftp_cleanup_pending"})
		return err
	}
	_, err = m.updateUploadJob(id, UpdateTransferJob{Action: TransferCancelAction})
	return err
}

func (m *TransferManager) Start(ctx context.Context, alias, id, remotePath string, options StartUploadOptions) (ResumableUpload, error) {
	if m.isClosed() {
		return ResumableUpload{}, ErrUnavailable
	}
	cleaned, err := resumablePath(id, remotePath)
	if err != nil || options.Size < 0 {
		return ResumableUpload{}, ErrInvalidTransfer
	}
	if options.Size > maxRegularFileTransferBytes {
		return ResumableUpload{}, ErrTransferTooLarge
	}
	unlock := m.lock(alias, cleaned)
	defer unlock()
	remote, err := m.transferRemote(ctx, alias, id, cleaned)
	if err != nil {
		return ResumableUpload{}, err
	}
	stopCancellation := m.watchRemoteCancellation(ctx, alias, id, cleaned, remote)
	defer stopCancellation()
	keepRemote := false
	defer func() {
		if !keepRemote {
			m.releaseRemote(alias, id, cleaned)
		}
	}()

	expected, err := expectedTargetRevision(ctx, remote, cleaned, options)
	if err != nil {
		return ResumableUpload{}, err
	}
	part := uploadPartPath(cleaned, id)
	info, err := remote.Lstat(part)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		file, createErr := remote.Create(part)
		if createErr != nil {
			return ResumableUpload{}, createErr
		}
		if closeErr := file.Close(); closeErr != nil {
			return ResumableUpload{}, closeErr
		}
		info, err = remote.Lstat(part)
	case err == nil && !info.Mode().IsRegular():
		return ResumableUpload{}, ErrNotRegularFile
	}
	if err != nil {
		return ResumableUpload{}, err
	}
	if info.Size() > options.Size {
		return ResumableUpload{}, ErrOffsetMismatch
	}
	keepRemote = true
	return ResumableUpload{ID: id, Path: cleaned, Offset: info.Size(), Size: options.Size, ExpectedRevision: expected}, nil
}

func (m *TransferManager) Append(ctx context.Context, alias, id, remotePath string, offset, total int64, contents []byte) (ResumableUpload, error) {
	if m.isClosed() {
		return ResumableUpload{}, ErrUnavailable
	}
	cleaned, err := resumablePath(id, remotePath)
	if err != nil || offset < 0 || total < 0 || int64(len(contents)) > total-offset {
		return ResumableUpload{}, ErrOffsetMismatch
	}
	unlock := m.lock(alias, cleaned)
	defer unlock()
	remote, err := m.transferRemote(ctx, alias, id, cleaned)
	if err != nil {
		return ResumableUpload{}, err
	}
	stopCancellation := m.watchRemoteCancellation(ctx, alias, id, cleaned, remote)
	defer stopCancellation()
	keepRemote := false
	defer func() {
		if !keepRemote {
			m.releaseRemote(alias, id, cleaned)
		}
	}()
	part := uploadPartPath(cleaned, id)
	info, err := remote.Lstat(part)
	if err != nil {
		return ResumableUpload{}, err
	}
	if !info.Mode().IsRegular() {
		return ResumableUpload{}, ErrNotRegularFile
	}
	if info.Size() != offset {
		return ResumableUpload{}, ErrOffsetMismatch
	}
	file, err := remote.OpenFile(part, os.O_WRONLY)
	if err != nil {
		return ResumableUpload{}, err
	}
	if _, err = file.Seek(offset, 0); err == nil {
		var written int64
		written, err = io.Copy(file, bytes.NewReader(contents))
		if err == nil && written != int64(len(contents)) {
			err = io.ErrShortWrite
		}
	}
	closeErr := file.Close()
	if err != nil {
		return ResumableUpload{}, err
	}
	if closeErr != nil {
		return ResumableUpload{}, closeErr
	}
	updated, err := remote.Lstat(part)
	if err != nil {
		return ResumableUpload{}, err
	}
	if updated.Size() != offset+int64(len(contents)) {
		return ResumableUpload{}, ErrOffsetMismatch
	}
	keepRemote = true
	return ResumableUpload{ID: id, Path: cleaned, Offset: updated.Size(), Size: total}, nil
}

func (m *TransferManager) Complete(ctx context.Context, alias, id, remotePath string, total int64, expectedRevision, sourceFingerprint string) (Transfer, error) {
	if m.isClosed() {
		return Transfer{}, ErrUnavailable
	}
	cleaned, err := resumablePath(id, remotePath)
	if err != nil || total < 0 || expectedRevision == "" || sourceFingerprint == "" {
		return Transfer{}, ErrInvalidTransfer
	}
	unlock := m.lock(alias, cleaned)
	defer unlock()
	remote, err := m.transferRemote(ctx, alias, id, cleaned)
	if err != nil {
		return Transfer{}, err
	}
	stopCancellation := m.watchRemoteCancellation(ctx, alias, id, cleaned, remote)
	defer stopCancellation()
	defer m.releaseRemote(alias, id, cleaned)
	part := uploadPartPath(cleaned, id)
	info, err := remote.Lstat(part)
	if err != nil {
		return Transfer{}, err
	}
	if !info.Mode().IsRegular() || info.Size() != total {
		return Transfer{}, ErrUploadIncomplete
	}
	source, err := remote.Open(part)
	if err != nil {
		return Transfer{}, err
	}
	actualFingerprint, fingerprintErr := SourceFingerprint(ctx, source, total)
	closeErr := source.Close()
	if fingerprintErr != nil {
		return Transfer{}, fingerprintErr
	}
	if closeErr != nil {
		return Transfer{}, closeErr
	}
	if actualFingerprint != sourceFingerprint {
		return Transfer{}, ErrConflict
	}
	verified, err := remote.Lstat(part)
	if err != nil || !verified.Mode().IsRegular() || verified.Size() != total || metadataRevision(verified) != metadataRevision(info) {
		if err != nil {
			return Transfer{}, err
		}
		return Transfer{}, ErrConflict
	}
	mode, err := verifyTargetRevision(ctx, remote, cleaned, expectedRevision)
	if err != nil {
		return Transfer{}, err
	}
	if err := remote.Chmod(part, mode); err != nil {
		return Transfer{}, err
	}
	publishedInfo, err := remote.Lstat(part)
	if err != nil {
		return Transfer{}, err
	}
	if !publishedInfo.Mode().IsRegular() || publishedInfo.Size() != total {
		return Transfer{}, ErrConflict
	}
	if err := remote.Replace(part, cleaned); err != nil {
		return Transfer{}, err
	}
	// Replace is the publication commit point. A diagnostic Lstat failure after
	// it must not turn an already-visible target into a failed, un-retryable job.
	revision := metadataRevision(publishedInfo)
	updated, err := remote.Lstat(cleaned)
	if err == nil {
		revision = metadataRevision(updated)
	}
	return Transfer{Path: cleaned, Bytes: total, Revision: revision}, nil
}

// SourceFingerprint hashes each 1 MiB source chunk independently and then
// hashes the big-endian source size followed by those chunk hashes. It matches
// the browser implementation without retaining the whole file in memory.
func SourceFingerprint(ctx context.Context, source io.Reader, size int64) (string, error) {
	if size < 0 {
		return "", ErrInvalidTransfer
	}
	summary := sha256.New()
	var encodedSize [8]byte
	binary.BigEndian.PutUint64(encodedSize[:], uint64(size))
	_, _ = summary.Write(encodedSize[:])
	buffer := make([]byte, 1<<20)
	remaining := size
	for remaining > 0 {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		want := int64(len(buffer))
		if remaining < want {
			want = remaining
		}
		if _, err := io.ReadFull(source, buffer[:want]); err != nil {
			return "", err
		}
		chunk := sha256.Sum256(buffer[:want])
		_, _ = summary.Write(chunk[:])
		remaining -= want
	}
	return "tree-sha256:" + hex.EncodeToString(summary.Sum(nil)), nil
}

func (m *TransferManager) Cancel(ctx context.Context, alias, id, remotePath string) error {
	if m.isClosed() {
		return ErrUnavailable
	}
	cleaned, err := resumablePath(id, remotePath)
	if err != nil {
		return err
	}
	unlock := m.lock(alias, cleaned)
	defer unlock()
	remote, err := m.transferRemote(ctx, alias, id, cleaned)
	if err != nil {
		return err
	}
	stopCancellation := m.watchRemoteCancellation(ctx, alias, id, cleaned, remote)
	defer stopCancellation()
	defer m.releaseRemote(alias, id, cleaned)
	err = remote.Remove(uploadPartPath(cleaned, id))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func expectedTargetRevision(ctx context.Context, remote Remote, target string, options StartUploadOptions) (string, error) {
	info, err := remote.Lstat(target)
	switch {
	case err == nil:
		if !info.Mode().IsRegular() {
			return "", ErrNotRegularFile
		}
		if !options.Overwrite {
			return "", ErrAlreadyExists
		}
		current, revisionErr := targetContentRevision(ctx, remote, target, info)
		if revisionErr != nil {
			return "", revisionErr
		}
		if options.ExpectedRevision != "" && options.ExpectedRevision != current {
			return "", ErrConflict
		}
		return current, nil
	case errors.Is(err, fs.ErrNotExist):
		if options.ExpectedRevision != "" && options.ExpectedRevision != AbsentRevision {
			return "", ErrConflict
		}
		return AbsentRevision, nil
	default:
		return "", err
	}
}

func verifyTargetRevision(ctx context.Context, remote Remote, target, expected string) (fs.FileMode, error) {
	info, err := remote.Lstat(target)
	if expected == AbsentRevision {
		if errors.Is(err, fs.ErrNotExist) {
			return 0o600, nil
		}
		if err != nil {
			return 0, err
		}
		return 0, ErrAlreadyExists
	}
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, ErrConflict
		}
		return 0, err
	}
	if !info.Mode().IsRegular() {
		return 0, ErrConflict
	}
	current, revisionErr := targetContentRevision(ctx, remote, target, info)
	if revisionErr != nil {
		return 0, revisionErr
	}
	if current != expected {
		return 0, ErrConflict
	}
	return info.Mode().Perm(), nil
}

func targetContentRevision(ctx context.Context, remote Remote, target string, before fs.FileInfo) (string, error) {
	source, err := remote.Open(target)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	written, copyErr := copyContext(ctx, hash, source, 0)
	closeErr := source.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	after, err := remote.Lstat(target)
	if err != nil {
		return "", err
	}
	if written != before.Size() || metadataRevision(before) != metadataRevision(after) {
		return "", ErrConflict
	}
	return "content-sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func resumablePath(id, remotePath string) (string, error) {
	if !transferIDPattern.MatchString(id) {
		return "", ErrInvalidTransfer
	}
	return cleanPublicPath(remotePath, false)
}

func uploadPartPath(target, id string) string {
	return path.Join(path.Dir(target), "."+path.Base(target)+".sshc-upload-"+id+".part")
}

func isUploadPartName(name string) bool {
	return uploadPartNamePattern.MatchString(name)
}

func isInternalName(name string) bool {
	return isUploadPartName(name) || editorTemporaryNamePattern.MatchString(name)
}

// LockOperation serializes every public mutation which can affect the given
// remote paths. Sorting canonical paths gives Rename a stable two-lock order
// and prevents two opposite renames from deadlocking.
func (m *TransferManager) LockOperation(alias string, remotePaths ...string) (func(), error) {
	if m == nil || m.Service == nil || m.isClosed() {
		return nil, ErrUnavailable
	}
	if err := validateAlias(alias); err != nil {
		return nil, err
	}
	unique := make(map[string]struct{}, len(remotePaths))
	paths := make([]string, 0, len(remotePaths))
	for _, remotePath := range remotePaths {
		cleaned, err := cleanPublicPath(remotePath, false)
		if err != nil {
			return nil, err
		}
		if _, exists := unique[cleaned]; exists {
			continue
		}
		unique[cleaned] = struct{}{}
		paths = append(paths, cleaned)
	}
	if len(paths) == 0 {
		return nil, ErrInvalidPath
	}
	sort.Strings(paths)
	unlocks := make([]func(), 0, len(paths))
	for _, remotePath := range paths {
		unlocks = append(unlocks, m.lock(alias, remotePath))
	}
	if m.isClosed() {
		for index := len(unlocks) - 1; index >= 0; index-- {
			unlocks[index]()
		}
		return nil, ErrUnavailable
	}
	return func() {
		for index := len(unlocks) - 1; index >= 0; index-- {
			unlocks[index]()
		}
	}, nil
}

func (m *TransferManager) lock(alias, target string) func() {
	return m.acquireLock(alias, target, false)
}

func (m *TransferManager) readLock(alias, target string) func() {
	return m.acquireLock(alias, target, true)
}

func (m *TransferManager) acquireLock(alias, target string, shared bool) func() {
	key := alias + "\x00" + target
	m.mutex.Lock()
	entry := m.locks[key]
	if entry == nil {
		entry = &transferLock{}
		m.locks[key] = entry
	}
	entry.refs++
	m.mutex.Unlock()
	if shared {
		entry.mutex.RLock()
	} else {
		entry.mutex.Lock()
	}
	return func() {
		if shared {
			entry.mutex.RUnlock()
		} else {
			entry.mutex.Unlock()
		}
		m.mutex.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(m.locks, key)
		}
		m.mutex.Unlock()
	}
}

func transferRemoteKey(alias, id, target string) string {
	return alias + "\x00" + id + "\x00" + target
}

func (m *TransferManager) transferRemote(ctx context.Context, alias, id, target string) (Remote, error) {
	key := transferRemoteKey(alias, id, target)
	m.mutex.Lock()
	if m.closed {
		m.mutex.Unlock()
		return nil, ErrUnavailable
	}
	remote := m.remotes[key]
	m.mutex.Unlock()
	if remote != nil {
		return remote, nil
	}
	remote, err := m.Service.open(ctx, alias)
	if err != nil {
		return nil, err
	}
	m.mutex.Lock()
	if m.closed {
		m.mutex.Unlock()
		_ = remote.Close()
		return nil, ErrUnavailable
	}
	if existing := m.remotes[key]; existing != nil {
		m.mutex.Unlock()
		_ = remote.Close()
		return existing, nil
	}
	m.remotes[key] = remote
	m.mutex.Unlock()
	return remote, nil
}

// watchRemoteCancellation closes and detaches a retained upload transport only
// when the current request is cancelled. Normal request completion stops the
// watcher and preserves the connection for the next chunk.
func (m *TransferManager) watchRemoteCancellation(ctx context.Context, alias, id, target string, remote Remote) func() {
	stop := context.AfterFunc(ctx, func() { m.releaseRemoteIf(alias, id, target, remote) })
	return func() { stop() }
}

// SaveText serializes editor publication with resumable upload publication
// for the same alias and target in this engine generation.
func (m *TransferManager) SaveText(ctx context.Context, alias, remotePath, contents, expectedRevision string) (TextFile, error) {
	if m == nil || m.Service == nil || m.isClosed() {
		return TextFile{}, ErrUnavailable
	}
	cleaned, err := cleanPublicPath(remotePath, false)
	if err != nil {
		return TextFile{}, err
	}
	unlock := m.lock(alias, cleaned)
	defer unlock()
	return m.Service.SaveText(ctx, alias, cleaned, contents, expectedRevision)
}

func (m *TransferManager) releaseRemote(alias, id, target string) {
	remote := m.detachRemote(alias, id, target)
	if remote != nil {
		_ = remote.Close()
	}
}

func (m *TransferManager) releaseRemoteIf(alias, id, target string, expected Remote) {
	key := transferRemoteKey(alias, id, target)
	m.mutex.Lock()
	remote := m.remotes[key]
	if remote == expected {
		delete(m.remotes, key)
	} else {
		remote = nil
	}
	m.mutex.Unlock()
	if remote != nil {
		_ = remote.Close()
	}
}

func (m *TransferManager) detachRemote(alias, id, target string) Remote {
	key := transferRemoteKey(alias, id, target)
	m.mutex.Lock()
	remote := m.remotes[key]
	delete(m.remotes, key)
	m.mutex.Unlock()
	return remote
}
