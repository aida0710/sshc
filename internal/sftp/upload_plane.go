package sftp

import (
	"context"
	"errors"
	"io/fs"
	"path"
	"time"
)

// StartOwned chooses sequential or ranged upload from the job's split
// settings and the host's current connection limit, and records the choice for
// the ranges and the completion that follow. A setting the job left to the
// engine is fixed in the job at its first start, so a change of the engine
// settings during the upload cannot refuse the chunks the client was told to
// send, nor a later resume of the same part. The owner lock serializes
// preparation with append, complete, pause and cancel; ranged writes later
// share this lock with each other.
func (m *TransferManager) StartOwned(ctx context.Context, alias, id, remotePath string, options StartUploadOptions) (ResumableUpload, error) {
	ctx = m.withTransferLimiter(ctx)
	unlock := m.lock("", jobOwnerLockKey(id))
	defer unlock()
	done, err := m.KeepJobActive(id)
	if err != nil {
		return ResumableUpload{}, err
	}
	defer done()
	target := UploadTarget{Alias: alias, ID: id, RemotePath: remotePath}
	if err := m.AuthorizeUpload(target, options.Size, false); err != nil {
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
		if err := m.recordSourceFingerprintLocked(record, options.SourceFingerprint); err != nil {
			m.jobsMutex.Unlock()
			return ResumableUpload{}, err
		}
	}
	job := record.job
	options.Overwrite = job.Overwrite
	options.ExpectedRevision = job.ExpectedRevision
	settings := m.largeFileSplitLocked(job)
	completed := append([]UploadRange(nil), job.UploadRanges...)
	acknowledged := acknowledgedUploadPrefix(job)
	m.jobsMutex.Unlock()
	split := m.uploadSplit(alias, settings)
	var upload ResumableUpload
	if split.divides(options.Size) {
		upload, err = m.startParallelUpload(ctx, target, options, completed)
		upload.Parallelism, upload.ChunkBytes = split.parallelism, split.chunkBytes
	} else {
		upload, err = m.startSequentialUpload(ctx, target, options, acknowledged)
		upload.Parallelism, upload.ChunkBytes = 1, split.chunkBytes
	}
	if err != nil {
		return ResumableUpload{}, err
	}
	total, offset := options.Size, upload.Offset
	m.jobsMutex.Lock()
	record = m.jobs[id]
	if record == nil || record.job.Status != TransferRunning {
		m.jobsMutex.Unlock()
		m.releaseRemote(alias, id, upload.Path)
		return ResumableUpload{}, ErrTransferState
	}
	err = m.rewriteJobLocked(record, true, func(now time.Time) error {
		record.uploadParallelism = upload.Parallelism
		record.job.LargeFileThresholdBytes = settings.threshold
		record.job.LargeFileParallelism = settings.parallelism
		record.job.LargeFileChunkBytes = settings.chunkBytes
		record.job.ExpectedRevision = upload.ExpectedRevision
		record.job.UploadRanges = append([]UploadRange(nil), upload.CompletedRanges...)
		record.job.TransferredBytes = offset
		record.job.TotalBytes = total
		record.job.BytesPerSecond, record.job.RemainingSeconds = 0, -1
		record.sampleAt, record.sampleBytes = now, offset
		return nil
	})
	if err != nil {
		m.jobsMutex.Unlock()
		m.releaseRemote(alias, id, upload.Path)
		return ResumableUpload{}, err
	}
	m.jobsMutex.Unlock()
	return upload, nil
}

// CompleteOwned publishes the part and commits the completed job before the
// request returns, so the remote file is never renamed into place while the
// job record still describes an unfinished upload.
func (m *TransferManager) CompleteOwned(ctx context.Context, completion UploadCompletion) (Transfer, error) {
	ctx = m.withTransferLimiter(ctx)
	alias, id, remotePath := completion.Target.Alias, completion.Target.ID, completion.Target.RemotePath
	total, expectedRevision, sourceFingerprint := completion.Total, completion.ExpectedRevision, completion.SourceFingerprint
	unlock := m.lock("", jobOwnerLockKey(id))
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
	if err := m.AuthorizeUpload(completion.Target, total, false); err != nil {
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
		if err := m.recordSourceFingerprintLocked(record, sourceFingerprint); err != nil {
			m.jobsMutex.Unlock()
			return Transfer{}, err
		}
	}
	preparedForRanges := record.uploadParallelism > 1
	uploaded := uploadRangeBytes(record.job.UploadRanges)
	m.jobsMutex.Unlock()
	if preparedForRanges && uploaded != total {
		return Transfer{}, ErrUploadIncomplete
	}
	transfer, err := m.completeUploadPart(ctx, completion)
	if err != nil {
		if errors.Is(err, ErrAmbiguousTransfer) {
			m.recordUncertainUpload(id)
		}
		return Transfer{}, err
	}
	if _, err := m.updateUploadJob(id, UpdateTransferJob{Action: TransferCompleteAction, TransferredBytes: &total}); err != nil {
		m.markTransferReconciliationRequired(nil, id, total)
		return Transfer{}, errors.Join(ErrAmbiguousTransfer, err)
	}
	return transfer, nil
}

// recordSourceFingerprintLocked は、送り元のファイルの指紋を、まだ無い記録に書く。
func (m *TransferManager) recordSourceFingerprintLocked(record *transferJobRecord, fingerprint string) error {
	return m.rewriteJobLocked(record, true, func(time.Time) error {
		record.job.SourceFingerprint = fingerprint
		return nil
	})
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
	unlock := m.lock("", jobOwnerLockKey(id))
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
		// still remove that exact part. cancelUploadPart validates both the
		// transfer ID and remote path and never addresses the published target.
		return m.cancelUploadPart(ctx, alias, id, remotePath)
	}
	if err != nil {
		return err
	}
	defer done()
	if err := m.AuthorizeUpload(UploadTarget{Alias: alias, ID: id, RemotePath: remotePath}, -1, true); err != nil {
		return err
	}
	if err := m.cancelUploadPart(ctx, alias, id, remotePath); err != nil {
		_, _ = m.updateUploadJob(id, UpdateTransferJob{Action: TransferFailAction, Problem: CleanupPendingProblem})
		return err
	}
	_, err = m.updateUploadJob(id, UpdateTransferJob{Action: TransferCancelAction})
	return err
}

// cancelUploadPart removes the part. Requests reach it through CancelOwned,
// and a failed upload's part is removed through cleanupTransferPart.
func (m *TransferManager) cancelUploadPart(ctx context.Context, alias, id, remotePath string) error {
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

func resumablePath(id, remotePath string) (string, error) {
	if !transferIDPattern.MatchString(id) {
		return "", ErrInvalidTransfer
	}
	return cleanPublicPath(remotePath, false)
}

func uploadPartPath(target, id string) string {
	return path.Join(path.Dir(target), "."+path.Base(target)+".sshc-upload-"+id+".part")
}

// AuthorizeUpload binds the data-plane upload endpoints to the queue record
// that owns their slot. Without this check, callers could append or complete a
// part file without ever acquiring one of the backend concurrency slots.
func (m *TransferManager) AuthorizeUpload(target UploadTarget, total int64, cancelling bool) error {
	id := target.ID
	cleaned, err := cleanPublicPath(target.RemotePath, false)
	if err != nil || !transferIDPattern.MatchString(id) {
		return ErrInvalidTransfer
	}
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	record := m.jobs[id]
	if record == nil {
		return ErrTransferNotFound
	}
	job := record.job
	if job.Direction != TransferUpload || job.Kind != TransferFile || job.Alias != target.Alias ||
		job.RemotePath != cleaned || (total >= 0 && job.TotalBytes != total) {
		return ErrConflict
	}
	if cancelling {
		return nil
	}
	if job.Status != TransferRunning {
		return ErrTransferState
	}
	return nil
}
