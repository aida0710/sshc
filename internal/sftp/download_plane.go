package sftp

import (
	"context"
	"os"
	"time"
)

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

// AuthorizeDownload binds a GET data-plane request to a running download job
// which already owns a shared queue slot.
func (m *TransferManager) AuthorizeDownload(id, alias, remotePath string, kind TransferKind) (TransferJob, error) {
	cleaned, err := cleanPublicPath(remotePath, false)
	if err != nil || !transferIDPattern.MatchString(id) {
		return TransferJob{}, ErrInvalidTransfer
	}
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	record := m.jobs[id]
	if record == nil {
		return TransferJob{}, ErrTransferNotFound
	}
	job := record.job
	if job.Direction != TransferDownload || job.Kind != kind || job.Alias != alias || job.RemotePath != cleaned {
		return TransferJob{}, ErrConflict
	}
	if job.Status != TransferRunning {
		return TransferJob{}, ErrTransferState
	}
	return job, nil
}

// StartDownloadDataPlane validates ownership and marks the operation active in
// one jobsMutex critical section. Cancel cannot slip between authorization and
// the stale-sweep protection token.
func (m *TransferManager) StartDownloadDataPlane(id, alias, remotePath string, kind TransferKind) (TransferJob, func(), error) {
	cleaned, err := cleanPublicPath(remotePath, false)
	if err != nil || !transferIDPattern.MatchString(id) {
		return TransferJob{}, nil, ErrInvalidTransfer
	}
	m.jobsMutex.Lock()
	m.initializeJobsLocked()
	record := m.jobs[id]
	if record == nil {
		m.jobsMutex.Unlock()
		return TransferJob{}, nil, ErrTransferNotFound
	}
	job := record.job
	if job.Direction != TransferDownload || job.Kind != kind || job.Alias != alias || job.RemotePath != cleaned {
		m.jobsMutex.Unlock()
		return TransferJob{}, nil, ErrConflict
	}
	if job.Status != TransferRunning {
		m.jobsMutex.Unlock()
		return TransferJob{}, nil, ErrTransferState
	}
	m.dataPlane[id]++
	m.jobsMutex.Unlock()
	done := func() {
		m.jobsMutex.Lock()
		if m.dataPlane[id] <= 1 {
			delete(m.dataPlane, id)
		} else {
			m.dataPlane[id]--
		}
		m.jobsMutex.Unlock()
	}
	return job, done, nil
}

// BeginDownload pins one response to an opaque content revision. A resumed
// response may start only at the browser's last durable checkpoint; bytes that
// were merely written to a socket are tracked separately in sentBytes.
func (m *TransferManager) BeginDownload(id string, total int64, revision string, offset int64) (TransferJob, error) {
	if !transferIDPattern.MatchString(id) || revision == "" || total < -1 || offset < 0 {
		return TransferJob{}, ErrInvalidTransfer
	}
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	record := m.jobs[id]
	if record == nil {
		return TransferJob{}, ErrTransferNotFound
	}
	job := &record.job
	if job.Direction != TransferDownload || job.Status != TransferRunning {
		return TransferJob{}, ErrTransferState
	}
	original := cloneTransferJobRecord(record)
	changed := record.revision != revision || (total >= 0 && job.TotalBytes != total)
	if changed {
		record.revision, record.sentBytes = revision, 0
		job.DownloadRevision = revision
		job.TransferredBytes = 0
		job.TotalBytes = total
		job.BytesPerSecond, job.RemainingSeconds = 0, -1
		record.sampleAt, record.sampleBytes = m.now().UTC(), 0
	}
	job.DownloadRevision = record.revision
	if offset != job.TransferredBytes || offset > record.sentBytes {
		return TransferJob{}, ErrOffsetMismatch
	}
	job.UpdatedAt = m.now().UTC()
	if err := m.persistJobsLocked(true); err != nil {
		*record = original
		return TransferJob{}, err
	}
	return *job, nil
}

// RecordDownloadSent records a server-observed socket high-water mark. It does
// not advance TransferredBytes: only a durable browser checkpoint ACK may do
// that, so a crash can safely request retransmission of an already-sent suffix.
func (m *TransferManager) RecordDownloadSent(id string, sent, total int64, revision string) (TransferJob, error) {
	if !transferIDPattern.MatchString(id) || revision == "" || sent < 0 || total < -1 {
		return TransferJob{}, ErrInvalidTransfer
	}
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	record := m.jobs[id]
	if record == nil {
		return TransferJob{}, ErrTransferNotFound
	}
	job := &record.job
	if job.Direction != TransferDownload || job.Status != TransferRunning || record.revision != revision {
		return TransferJob{}, ErrTransferState
	}
	original := cloneTransferJobRecord(record)
	if total >= 0 {
		if sent > total {
			return TransferJob{}, ErrOffsetMismatch
		}
		job.TotalBytes = total
	}
	if sent > record.sentBytes {
		record.sentBytes = sent
	}
	job.UpdatedAt = m.now().UTC()
	if err := m.persistJobsLocked(false); err != nil {
		*record = original
		return TransferJob{}, err
	}
	return *job, nil
}

// VerifyDownloadComplete confirms that an earlier response from this engine
// actually sent the whole pinned revision. A metadata-only verify request must
// never manufacture sentBytes after an engine restart.
func (m *TransferManager) VerifyDownloadComplete(id string, total int64, revision string) (TransferJob, error) {
	if !transferIDPattern.MatchString(id) || revision == "" || total < 0 {
		return TransferJob{}, ErrInvalidTransfer
	}
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	record := m.jobs[id]
	if record == nil {
		return TransferJob{}, ErrTransferNotFound
	}
	job := &record.job
	if job.Direction != TransferDownload || job.Status != TransferRunning || record.revision != revision {
		return TransferJob{}, ErrTransferState
	}
	if job.TotalBytes != total || record.sentBytes != total {
		return TransferJob{}, ErrOffsetMismatch
	}
	original := cloneTransferJobRecord(record)
	job.UpdatedAt = m.now().UTC()
	if err := m.persistJobsLocked(true); err != nil {
		*record = original
		return TransferJob{}, err
	}
	return *job, nil
}

// AcknowledgeDownload advances (or reconciles backwards) to an OPFS-durable
// offset only when it belongs to the currently pinned response revision and is
// no larger than bytes the server actually sent.
func (m *TransferManager) AcknowledgeDownload(id string, offset int64, revision string) (TransferJob, error) {
	if !transferIDPattern.MatchString(id) || revision == "" || offset < 0 {
		return TransferJob{}, ErrInvalidTransfer
	}
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	record := m.jobs[id]
	if record == nil {
		return TransferJob{}, ErrTransferNotFound
	}
	job := &record.job
	if job.Direction != TransferDownload || job.Status != TransferRunning || record.revision != revision {
		return TransferJob{}, ErrTransferState
	}
	if offset > record.sentBytes || (job.TotalBytes >= 0 && offset > job.TotalBytes) {
		return TransferJob{}, ErrOffsetMismatch
	}
	original := cloneTransferJobRecord(record)
	now := m.now().UTC()
	if offset < job.TransferredBytes {
		job.BytesPerSecond, job.RemainingSeconds = 0, -1
		record.sampleAt, record.sampleBytes = now, offset
	}
	job.TransferredBytes = offset
	m.updateRateLocked(record, now)
	job.UpdatedAt = now
	if err := m.persistJobsLocked(false); err != nil {
		*record = original
		return TransferJob{}, err
	}
	return *job, nil
}
