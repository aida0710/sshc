package sftp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"io/fs"
)

// remoteFileCheckpoint is immutable after admission to the copy. Offset is
// recovered from the actual part, whose complete prefix must match the source.
// Content hashing also detects equal-size, equal-mtime source replacement.
type remoteFileCheckpoint struct {
	SourceRevision        string
	SourceContentRevision string
	TargetRevision        string
}

func (m *TransferManager) copyServerFile(run *remoteRun, job TransferJob) (operationErr error) {
	unlock := m.lock("", "\x00server-file:"+job.ID)
	defer unlock()
	if err := run.ownershipError(); err != nil {
		return err
	}
	source, target, err := m.openServerFileEnds(run.ctx, job)
	if err != nil {
		return err
	}
	defer func() {
		source.close(operationErr)
		if target.remote != source.remote || target.root != nil {
			target.close(operationErr)
		}
	}()
	before, err := source.stat(source.name)
	if err != nil {
		return err
	}
	if before.Size() < 0 || before.Size() > maxRegularFileTransferBytes {
		return ErrTransferTooLarge
	}
	if job.TotalBytes >= 0 && before.Size() != job.TotalBytes {
		return ErrConflict
	}
	if job.Operation == RemoteCopy {
		ends := remoteTransferEnds{source: source.remote, target: target.remote, sourcePath: source.name, targetPath: target.name, sameAlias: job.SourceAlias == job.Alias, overwrite: job.Overwrite}
		if err := m.Service.refuseTransferOntoSource(ends, before); err != nil {
			return err
		}
	}
	checkpoint, err := m.prepareRemoteFileCheckpoint(run, job, remoteFileCheckpointRequest{source: source, target: target, before: before})
	if err != nil {
		return err
	}
	partInfo, err := target.stat(target.part)
	// Connection loss can happen after the checkpoint is durable but before
	// creation. With no acknowledged payload, recreating an absent part is safe.
	create := errors.Is(err, fs.ErrNotExist) && job.TransferredBytes == 0
	if err != nil && !create {
		return err
	}
	var offset int64
	if !create {
		offset = partInfo.Size()
		if offset < job.TransferredBytes || offset > before.Size() {
			return ErrConflict
		}
		if err := verifyServerFilePrefix(run.ctx, serverFilePrefix{source: source, target: target, offset: offset}); err != nil {
			return err
		}
	}
	output, err := target.openPart(create)
	if err != nil {
		return err
	}
	defer output.Close()
	if _, err := output.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	input, err := source.open(source.name, offset)
	if err != nil {
		return err
	}
	defer input.Close()
	if err := m.reportServerFileOffset(run, job.ID, offset); err != nil {
		return err
	}
	destination := io.Writer(output)
	if target.remote != nil {
		destination = m.Service.transferWriter(run.ctx, output)
	}
	writer := &progressWriter{Writer: destination, report: func(delta int64) error {
		offset += delta
		return m.reportServerFileOffset(run, job.ID, offset)
	}}
	reader := io.Reader(input)
	if source.remote != nil {
		reader = &limitedTransferReader{ctx: run.ctx, source: input, limiter: m.limiter}
	}
	_, err = copyContext(run.ctx, writer, io.LimitReader(reader, before.Size()-offset), 0)
	if err != nil {
		return err
	}
	if offset != before.Size() {
		return ErrConflict
	}
	if file, ok := output.(interface{ Sync() error }); ok {
		if err := file.Sync(); err != nil {
			return err
		}
	}
	if err := output.Close(); err != nil {
		return err
	}
	after, err := source.stat(source.name)
	if err != nil {
		return err
	}
	if metadataRevision(after) != checkpoint.SourceRevision {
		return ErrConflict
	}
	if digest, err := serverFileDigest(run.ctx, source); err != nil {
		return err
	} else if digest != checkpoint.SourceContentRevision {
		return ErrConflict
	}
	if err := verifyServerFilePrefix(run.ctx, serverFilePrefix{source: source, target: target, offset: offset}); err != nil {
		return err
	}
	if revision, err := target.revision(); err != nil {
		return err
	} else if revision != checkpoint.TargetRevision {
		return ErrConflict
	}
	if err := run.ctx.Err(); err != nil {
		return err
	}
	// Once publication is attempted, a missing acknowledgement cannot prove
	// whether rename succeeded. Even a transport failure must stop here.
	if job.Operation == RemoteCopy {
		if err := target.remote.Chmod(target.part, before.Mode().Perm()); err != nil {
			return err
		}
	}
	if err := target.publish(job.Overwrite, before.ModTime()); err != nil {
		return err
	}
	return nil
}

type remoteFileCheckpointRequest struct {
	source *serverFileEnd
	target *serverFileEnd
	before fs.FileInfo
}

func (m *TransferManager) prepareRemoteFileCheckpoint(run *remoteRun, job TransferJob, request remoteFileCheckpointRequest) (*remoteFileCheckpoint, error) {
	source, target, before := request.source, request.target, request.before
	digest, err := serverFileDigest(run.ctx, source)
	if err != nil {
		return nil, err
	}
	revision, err := target.revision()
	if err != nil {
		return nil, err
	}
	if existing := job.RemoteCheckpoint; existing != nil {
		if existing.SourceRevision != metadataRevision(before) || existing.SourceContentRevision != digest || existing.TargetRevision != revision {
			return nil, ErrConflict
		}
		return existing, nil
	}
	if revision != AbsentRevision && !job.Overwrite {
		return nil, ErrAlreadyExists
	}
	checkpoint := &remoteFileCheckpoint{SourceRevision: metadataRevision(before), SourceContentRevision: digest, TargetRevision: revision}
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	record := m.jobs[job.ID]
	if record == nil || record.job.Status != TransferRunning {
		return nil, ErrTransferState
	}
	if err := run.ownershipError(); err != nil {
		return nil, err
	}
	original := cloneTransferJobRecord(record)
	record.job.RemoteCheckpoint = checkpoint
	if err := m.persistJobsLocked(true); err != nil {
		*record = original
		return nil, err
	}
	return checkpoint, nil
}

func (m *TransferManager) reportServerFileOffset(run *remoteRun, id string, offset int64) error {
	_, err := m.reportRemoteRun(run, id, UpdateTransferJob{Action: TransferProgressAction, TransferredBytes: &offset})
	return err
}

func serverFileDigest(ctx context.Context, source *serverFileEnd) (string, error) {
	file, err := source.open(source.name, 0)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	reader := io.Reader(file)
	if source.remote != nil {
		reader = &limitedTransferReader{ctx: ctx, source: file, limiter: source.service.transferLimiter}
	}
	if _, err := copyContext(ctx, digest, reader, maxRegularFileTransferBytes); err != nil {
		return "", err
	}
	return contentRevisionOf(digest), nil
}

type serverFilePrefix struct {
	source *serverFileEnd
	target *serverFileEnd
	offset int64
}

func verifyServerFilePrefix(ctx context.Context, prefix serverFilePrefix) error {
	if prefix.offset == 0 {
		return nil
	}
	source, err := prefix.source.open(prefix.source.name, 0)
	if err != nil {
		return err
	}
	defer source.Close()
	part, err := prefix.target.open(prefix.target.part, 0)
	if err != nil {
		return err
	}
	defer part.Close()
	// Fixed-size buffers make verification independent of the partial file's size.
	var sourceBytes, partBytes [transferSpeedQuantum]byte
	remaining := prefix.offset
	for remaining > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		size := int(min(remaining, int64(len(sourceBytes))))
		if _, err := io.ReadFull(serverFileReader(ctx, prefix.source, source), sourceBytes[:size]); err != nil {
			return err
		}
		if _, err := io.ReadFull(serverFileReader(ctx, prefix.target, part), partBytes[:size]); err != nil {
			return err
		}
		if !bytes.Equal(sourceBytes[:size], partBytes[:size]) {
			return ErrConflict
		}
		remaining -= int64(size)
	}
	return nil
}

func serverFileReader(ctx context.Context, end *serverFileEnd, reader io.Reader) io.Reader {
	if end.remote == nil {
		return reader
	}
	return &limitedTransferReader{ctx: ctx, source: reader, limiter: end.service.transferLimiter}
}
