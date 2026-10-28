package sftp

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"time"
)

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
	err = m.rewriteJobLocked(record, true, func(now time.Time) error {
		record.job.UploadRanges = addUploadRange(record.job.UploadRanges, write.Range)
		record.job.TransferredBytes = uploadRangeBytes(record.job.UploadRanges)
		m.updateRateLocked(record, now)
		return nil
	})
	if err != nil {
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
	return m.rewriteJobLocked(record, true, func(now time.Time) error {
		record.job.UploadRanges = nil
		record.job.TransferredBytes = 0
		record.job.BytesPerSecond, record.job.RemainingSeconds = 0, -1
		record.sampleAt, record.sampleBytes = now, 0
		return nil
	})
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
