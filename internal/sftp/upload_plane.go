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
	"sort"
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
		original := cloneTransferJobRecord(record)
		record.job.SourceFingerprint = options.SourceFingerprint
		record.job.UpdatedAt = m.now().UTC()
		if err := m.persistJobsLocked(true); err != nil {
			*record = original
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
	original := cloneTransferJobRecord(record)
	record.uploadParallelism = upload.Parallelism
	record.job.LargeFileThresholdBytes = settings.threshold
	record.job.LargeFileParallelism = settings.parallelism
	record.job.LargeFileChunkBytes = settings.chunkBytes
	record.job.ExpectedRevision = upload.ExpectedRevision
	record.job.UploadRanges = append([]UploadRange(nil), upload.CompletedRanges...)
	record.job.TransferredBytes = offset
	record.job.TotalBytes = total
	record.job.BytesPerSecond, record.job.RemainingSeconds = 0, -1
	record.sampleAt, record.sampleBytes = m.now().UTC(), offset
	record.job.UpdatedAt = record.sampleAt
	if err := m.persistJobsLocked(true); err != nil {
		*record = original
		m.jobsMutex.Unlock()
		m.releaseRemote(alias, id, upload.Path)
		return ResumableUpload{}, err
	}
	m.jobsMutex.Unlock()
	return upload, nil
}

// AppendRangeOwned streams one complete configured range to an independent
// SFTP connection. Successful ranges are persisted and coalesced, so an
// interrupted browser or CLI only resends ranges whose acknowledgement was
// not made durable. Ranges are accepted only while the part is prepared for
// them by StartOwned, even if the host's connection limit changed since.
func (m *TransferManager) AppendRangeOwned(ctx context.Context, write UploadRangeWrite) (ResumableUpload, error) {
	id, total := write.Target.ID, write.Total
	offset, length := write.Range.Offset, write.Range.Size
	unlock := m.readLock("", jobOwnerLockKey(id))
	defer unlock()
	done, err := m.KeepJobActive(id)
	if err != nil {
		return ResumableUpload{}, err
	}
	defer done()
	if err := m.AuthorizeUpload(write.Target, total, false); err != nil {
		return ResumableUpload{}, err
	}
	m.jobsMutex.Lock()
	record := m.jobs[id]
	if record == nil {
		m.jobsMutex.Unlock()
		return ResumableUpload{}, ErrTransferNotFound
	}
	chunkBytes := m.largeFileSplitLocked(record.job).chunkBytes
	parallelism := record.uploadParallelism
	expectedRevision := record.job.ExpectedRevision
	already := uploadRangeCovered(record.job.UploadRanges, offset, length)
	m.jobsMutex.Unlock()
	if parallelism < 2 || offset < 0 || length <= 0 || offset%chunkBytes != 0 ||
		length != min(chunkBytes, total-offset) || offset+length > total {
		return ResumableUpload{}, ErrInvalidTransfer
	}
	if !already {
		if err := m.writeUploadRange(ctx, write); err != nil {
			return ResumableUpload{}, err
		}
	} else if err := consumeExactUploadRange(ctx, io.Discard, write.Contents, length); err != nil {
		return ResumableUpload{}, err
	}
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	record = m.jobs[id]
	if record == nil || record.job.Status != TransferRunning || record.job.ExpectedRevision != expectedRevision {
		return ResumableUpload{}, ErrTransferState
	}
	original := cloneTransferJobRecord(record)
	record.job.UploadRanges = addUploadRange(record.job.UploadRanges, write.Range)
	record.job.TransferredBytes = uploadRangeBytes(record.job.UploadRanges)
	now := m.now().UTC()
	m.updateRateLocked(record, now)
	record.job.UpdatedAt = now
	if err := m.persistJobsLocked(true); err != nil {
		*record = original
		return ResumableUpload{}, err
	}
	return ResumableUpload{ID: id, Path: path.Clean(write.Target.RemotePath), Offset: record.job.TransferredBytes, Size: total,
		ExpectedRevision: expectedRevision, CompletedRanges: append([]UploadRange(nil), record.job.UploadRanges...),
		Parallelism: parallelism, ChunkBytes: chunkBytes}, nil
}

// startParallelUpload prepares the part file that split ranges write into.
// The caller fills in Parallelism and ChunkBytes from the job's split.
func (m *TransferManager) startParallelUpload(
	ctx context.Context, target UploadTarget, options StartUploadOptions, completed []UploadRange,
) (_ ResumableUpload, operationErr error) {
	alias, id := target.Alias, target.ID
	cleaned, err := resumablePath(id, target.RemotePath)
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
	defer func() { closeTransferRemote(remote, operationErr) }()
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
		ExpectedRevision: expected, CompletedRanges: append([]UploadRange(nil), completed...)}, nil
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
	original := cloneTransferJobRecord(record)
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

func (m *TransferManager) writeUploadRange(ctx context.Context, write UploadRangeWrite) (operationErr error) {
	cleaned, err := resumablePath(write.Target.ID, write.Target.RemotePath)
	if err != nil {
		return err
	}
	unlock := m.readLock(write.Target.Alias, cleaned)
	defer unlock()
	remote, err := m.Service.openRequest(ctx, write.Target.Alias)
	if err != nil {
		return err
	}
	defer func() { closeTransferRemote(remote, operationErr) }()
	part := uploadPartPath(cleaned, write.Target.ID)
	info, err := remote.Lstat(part)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() != write.Total {
		return ErrOffsetMismatch
	}
	file, err := remote.OpenFile(part, os.O_WRONLY)
	if err != nil {
		return err
	}
	if _, err = file.Seek(write.Range.Offset, io.SeekStart); err == nil {
		err = consumeExactUploadRange(ctx, m.Service.transferWriter(ctx, file), write.Contents, write.Range.Size)
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
func (m *TransferManager) AppendOwned(ctx context.Context, chunk UploadAppend) (ResumableUpload, error) {
	alias, id, remotePath := chunk.Target.Alias, chunk.Target.ID, chunk.Target.RemotePath
	offset, total := chunk.Offset, chunk.Total
	unlock := m.lock("", jobOwnerLockKey(id))
	defer unlock()
	done, err := m.KeepJobActive(id)
	if err != nil {
		return ResumableUpload{}, err
	}
	defer done()
	if err := m.AuthorizeUpload(chunk.Target, total, false); err != nil {
		return ResumableUpload{}, err
	}
	if acknowledged, ok, err := m.replayAcknowledgedAppend(id, alias, remotePath, offset, total, len(chunk.Contents)); err != nil {
		return ResumableUpload{}, err
	} else if ok {
		return acknowledged, nil
	}
	upload, err := m.appendUploadPart(ctx, chunk)
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
		upload.ChunkBytes = m.largeFileSplitLocked(record.job).chunkBytes
	}
	m.jobsMutex.Unlock()
	upload.Parallelism = 1
	upload.CompletedRanges = []UploadRange{}
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
		original := cloneTransferJobRecord(record)
		record.job.SourceFingerprint = sourceFingerprint
		record.job.UpdatedAt = m.now().UTC()
		if err := m.persistJobsLocked(true); err != nil {
			*record = original
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
	return ResumableUpload{
		ID: id, Path: cleaned, Offset: end, Size: total, ExpectedRevision: record.job.ExpectedRevision,
		CompletedRanges: []UploadRange{}, Parallelism: 1, ChunkBytes: m.largeFileSplitLocked(record.job).chunkBytes,
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

// startSequentialUpload prepares a part written as one stream. A part that was
// prepared for ranges already has the final size and may have holes between
// them, so a full-size part is cut back to acknowledged, the leading bytes its
// job recorded as written, before one stream continues from its end.
func (m *TransferManager) startSequentialUpload(
	ctx context.Context, target UploadTarget, options StartUploadOptions, acknowledged int64,
) (_ ResumableUpload, operationErr error) {
	if m.isClosed() {
		return ResumableUpload{}, ErrUnavailable
	}
	alias, id := target.Alias, target.ID
	cleaned, err := resumablePath(id, target.RemotePath)
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
		if keepRemote {
			m.keepRemoteIdle(alias, id, cleaned, remote)
		} else {
			m.releaseRemoteAfterTransfer(target, operationErr)
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
	offset := info.Size()
	if offset == options.Size && acknowledged < offset {
		if err := truncateRemoteFile(remote, part, acknowledged); err != nil {
			return ResumableUpload{}, err
		}
		offset = acknowledged
	}
	keepRemote = true
	return ResumableUpload{ID: id, Path: cleaned, Offset: offset, Size: options.Size, ExpectedRevision: expected}, nil
}

// acknowledgedUploadPrefix is how many leading bytes of the job's part the job
// has recorded as written: the progress of one stream, or the range that
// starts the file when the part was written as ranges.
func acknowledgedUploadPrefix(job TransferJob) int64 {
	if len(job.UploadRanges) == 0 {
		return job.TransferredBytes
	}
	if leading := job.UploadRanges[0]; leading.Offset == 0 {
		return leading.Size
	}
	return 0
}

func truncateRemoteFile(remote Remote, name string, size int64) error {
	file, err := remote.OpenFile(name, os.O_WRONLY)
	if err != nil {
		return err
	}
	err = file.Truncate(size)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}

// appendUploadPart writes contents at offset in the part. It and the other
// *UploadPart functions check a request against the part only, not against the
// job's state, its owner lock or its queue slot. Requests reach it through
// AppendOwned, which checks those first.
func (m *TransferManager) appendUploadPart(ctx context.Context, chunk UploadAppend) (_ ResumableUpload, operationErr error) {
	alias, id, remotePath := chunk.Target.Alias, chunk.Target.ID, chunk.Target.RemotePath
	offset, total, contents := chunk.Offset, chunk.Total, chunk.Contents
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
		if keepRemote {
			m.keepRemoteIdle(alias, id, cleaned, remote)
		} else {
			m.releaseRemoteAfterTransfer(chunk.Target, operationErr)
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
		written, err = io.Copy(m.Service.transferWriter(ctx, file), bytes.NewReader(contents))
		if err == nil && written != int64(len(contents)) {
			err = io.ErrShortWrite
		}
	}
	if err != nil {
		// Pipelined writes can land bytes beyond the last one that succeeded.
		// The part must end at the acknowledged offset for the next append,
		// and for a resume, to continue from a prefix that really arrived.
		_ = file.Truncate(offset)
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

// completeUploadPart publishes a part that holds every byte and matches the
// source fingerprint. Requests reach it through CompleteOwned.
func (m *TransferManager) completeUploadPart(ctx context.Context, completion UploadCompletion) (_ Transfer, operationErr error) {
	alias, id, remotePath := completion.Target.Alias, completion.Target.ID, completion.Target.RemotePath
	total, expectedRevision, sourceFingerprint := completion.Total, completion.ExpectedRevision, completion.SourceFingerprint
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
	defer func() { m.releaseRemoteAfterTransfer(completion.Target, operationErr) }()
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
	actualFingerprint, fingerprintErr := SourceFingerprint(ctx, transferVerificationReader(ctx, source), total)
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
	if mode.replaceExisting {
		if err := remote.Chmod(part, mode.perm); err != nil {
			return Transfer{}, err
		}
	}
	if modified := m.jobLastModified(id); !modified.IsZero() {
		if err := remote.Chtimes(part, modified); err != nil {
			return Transfer{}, err
		}
	}
	publishedInfo, err := remote.Lstat(part)
	if err != nil {
		return Transfer{}, err
	}
	if !publishedInfo.Mode().IsRegular() || publishedInfo.Size() != total {
		return Transfer{}, ErrConflict
	}
	if err := publishUploadPart(remote, part, cleaned, expectedRevision); err != nil {
		return Transfer{}, publicationFailure(err)
	}
	// Publication is the commit point. A diagnostic Lstat failure after it must
	// not turn an already-visible target into a failed, un-retryable job.
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

// publishedMode is what the published upload's permissions become: those of
// the file it replaces, or, for a new file, whatever the server gave the part
// (its umask), as WinSCP leaves it by default.
type publishedMode struct {
	replaceExisting bool
	perm            fs.FileMode
}

func verifyTargetRevision(ctx context.Context, remote Remote, target, expected string) (publishedMode, error) {
	info, err := remote.Lstat(target)
	if expected == AbsentRevision {
		if errors.Is(err, fs.ErrNotExist) {
			return publishedMode{}, nil
		}
		if err != nil {
			return publishedMode{}, err
		}
		return publishedMode{}, ErrAlreadyExists
	}
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return publishedMode{}, ErrConflict
		}
		return publishedMode{}, err
	}
	if !info.Mode().IsRegular() {
		return publishedMode{}, ErrConflict
	}
	current, revisionErr := targetContentRevision(ctx, remote, target, info)
	if revisionErr != nil {
		return publishedMode{}, revisionErr
	}
	if current != expected {
		return publishedMode{}, ErrConflict
	}
	return publishedMode{replaceExisting: true, perm: info.Mode().Perm()}, nil
}

// jobLastModified is the modification time the source had, when the client
// reported one; the published file is given the same time.
func (m *TransferManager) jobLastModified(id string) time.Time {
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	record := m.jobs[id]
	if record == nil || record.job.LastModified <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(record.job.LastModified).UTC()
}

// publishUploadPart は part を target 名へ動かす。新規 file（expected が absent）は
// put／copy と同じく標準 rename を使い、verifyTargetRevision の後に別 client が同名を
// 作っていれば OpenSSH がその場で拒否する。上書きだけが posix-rename で置換する。
func publishUploadPart(remote Remote, part, target, expectedRevision string) error {
	if expectedRevision == AbsentRevision {
		return remote.Rename(part, target)
	}
	return remote.Replace(part, target)
}

func targetContentRevision(ctx context.Context, remote Remote, target string, before fs.FileInfo) (string, error) {
	source, err := remote.Open(target)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	written, copyErr := copyContext(ctx, hash, transferVerificationReader(ctx, source), 0)
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
	return contentRevisionOf(hash), nil
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
