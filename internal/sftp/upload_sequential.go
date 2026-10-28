package sftp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
)

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
