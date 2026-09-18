package sftp

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"sync"
)

// Single-file and directory downloads served to the browser: a prepared
// download is a bounded, resumable reader over a remote file or a ZIP
// spooled to a temporary file, copied out in one stream or in ranges.

type archiveBudget struct {
	entries int
	bytes   int64
}

// PreparedDownload is an immutable local spool of one remote-file read. Its
// revision hashes the exact bytes that will be sent, rather than SFTP v3's
// second-resolution metadata. This prevents equal-size, same-mtime remote
// replacements from being joined to an older downloaded prefix.
type PreparedDownload struct {
	file     *os.File
	name     string
	remove   bool
	lease    *preparedSpoolLease
	Size     int64
	Revision string
}

func (download *PreparedDownload) Close() error {
	if download == nil || download.file == nil {
		return nil
	}
	err := download.file.Close()
	var removeErr error
	if download.remove {
		removeErr = os.Remove(download.name)
	} else if download.lease != nil {
		download.lease.release()
	}
	download.file = nil
	return errors.Join(err, removeErr)
}

type boundedArchiveWriter struct {
	destination io.Writer
	remaining   int64
}

func (writer *boundedArchiveWriter) Write(contents []byte) (int, error) {
	if int64(len(contents)) > writer.remaining {
		return 0, ErrTransferTooLarge
	}
	written, err := writer.destination.Write(contents)
	writer.remaining -= int64(written)
	return written, err
}

func (s Service) prepareArchive(ctx context.Context, alias, remotePath, temporaryDirectory string, maxBytes int64) (_ *PreparedDownload, resultErr error) {
	if maxBytes <= 0 {
		return nil, ErrInvalidTransfer
	}
	temporary, err := os.CreateTemp(temporaryDirectory, "archive-*.part")
	if err != nil {
		return nil, err
	}
	prepared := &PreparedDownload{file: temporary, name: temporary.Name(), remove: true}
	defer func() {
		if resultErr != nil {
			_ = prepared.Close()
		}
	}()
	hash := sha256.New()
	destination := &boundedArchiveWriter{destination: io.MultiWriter(temporary, hash), remaining: maxBytes}
	if _, err := s.DownloadArchive(ctx, alias, remotePath, destination); err != nil {
		return nil, err
	}
	if err := temporary.Sync(); err != nil {
		return nil, err
	}
	info, err := temporary.Stat()
	if err != nil {
		return nil, err
	}
	prepared.Size = info.Size()
	prepared.Revision = contentRevisionOf(hash)
	return prepared, nil
}

func (download *PreparedDownload) WriteFrom(ctx context.Context, offset int64, destination io.Writer) (int64, error) {
	if download == nil || download.file == nil || offset < 0 || offset > download.Size {
		return 0, ErrOffsetMismatch
	}
	if _, err := download.file.Seek(offset, io.SeekStart); err != nil {
		return 0, err
	}
	return copyContext(ctx, destination, io.LimitReader(download.file, download.Size-offset), 0)
}

func (s Service) PrepareDownload(ctx context.Context, alias, remotePath string) (_ *PreparedDownload, resultErr error) {
	return s.prepareDownload(ctx, alias, remotePath, "", nil, 0, 1, 0, nil)
}

func (s Service) prepareDownload(
	ctx context.Context, alias, remotePath, temporaryDirectory string, reserve func(int64) error,
	splitThreshold int64, splitParallelism int, splitChunkBytes int64, progress func(DownloadPartProgress),
) (_ *PreparedDownload, resultErr error) {
	cleaned, err := cleanPublicPath(remotePath, false)
	if err != nil {
		return nil, err
	}
	remote, err := s.openRequest(ctx, alias)
	if err != nil {
		return nil, err
	}
	defer remote.Close()
	// A symlink downloads the file it points to, under the link's own name.
	file, err := locateFile(remote, cleaned)
	if err != nil {
		return nil, err
	}
	cleaned = file.stored
	before, err := remote.Lstat(cleaned)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, ErrNotRegularFile
	}
	if before.Size() < 0 || before.Size() > maxRegularFileTransferBytes {
		return nil, ErrTransferTooLarge
	}
	// Reserve the complete known size before opening or creating a spool. This
	// makes the process-wide disk quota a hard bound even with concurrent jobs.
	if reserve != nil {
		if err := reserve(before.Size()); err != nil {
			return nil, err
		}
	}
	temporary, err := os.CreateTemp(temporaryDirectory, "download-*.part")
	if err != nil {
		return nil, err
	}
	prepared := &PreparedDownload{file: temporary, name: temporary.Name(), remove: true}
	defer func() {
		if resultErr != nil {
			_ = prepared.Close()
		}
	}()
	written := int64(0)
	if before.Size() >= splitThreshold && splitThreshold > 0 && splitParallelism > 1 && splitChunkBytes > 0 {
		if _, supported := remote.(RangeRemote); supported {
			written, err = s.copyDownloadRanges(ctx, remote, alias, cleaned, temporary, before.Size(), splitParallelism, splitChunkBytes, progress)
		}
	}
	if written == 0 && before.Size() > 0 && err == nil {
		written, err = copyDownloadSequential(ctx, remote, cleaned, temporary, before.Size(), progress)
	}
	if err != nil {
		return nil, err
	}
	after, err := remote.Lstat(cleaned)
	if err != nil {
		return nil, err
	}
	if written != before.Size() || metadataRevision(before) != metadataRevision(after) {
		return nil, ErrConflict
	}
	if err := temporary.Sync(); err != nil {
		return nil, err
	}
	if _, err := temporary.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	hash := sha256.New()
	if hashed, err := copyContext(ctx, hash, temporary, 0); err != nil {
		return nil, err
	} else if hashed != written {
		return nil, ErrConflict
	}
	prepared.Size = written
	prepared.Revision = contentRevisionOf(hash)
	return prepared, nil
}

func copyDownloadSequential(
	ctx context.Context, remote Remote, remotePath string, destination *os.File, size int64,
	progress func(DownloadPartProgress),
) (int64, error) {
	source, err := remote.Open(remotePath)
	if err != nil {
		return 0, err
	}
	defer source.Close()
	if progress != nil {
		progress(DownloadPartProgress{Index: 0, TotalBytes: size})
	}
	output := io.Writer(destination)
	if progress != nil {
		output = &downloadProgressWriter{destination: destination, progress: func(written int64) {
			progress(DownloadPartProgress{Index: 0, TransferredBytes: written, TotalBytes: size})
		}}
	}
	var written int64
	if fastSource, ok := source.(io.WriterTo); ok {
		// pkg/sftp pipelines reads only through File.WriteTo. Wrapping the
		// source in LimitReader/copyContext forces one 32 KiB request per RTT.
		// Bound the writer instead so the pipelined path remains available.
		written, err = fastSource.WriteTo(&boundedContextWriter{ctx: ctx, destination: output, remaining: size})
	} else {
		written, err = copyContext(ctx, output, io.LimitReader(source, size), 0)
	}
	if err != nil {
		return 0, err
	}
	var extra [1]byte
	extraBytes, extraErr := source.Read(extra[:])
	if extraBytes != 0 {
		return 0, ErrConflict
	}
	if extraErr != nil && !errors.Is(extraErr, io.EOF) {
		return 0, extraErr
	}
	return written, nil
}

type boundedContextWriter struct {
	ctx         context.Context
	destination io.Writer
	remaining   int64
}

func (w *boundedContextWriter) Write(contents []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	if int64(len(contents)) > w.remaining {
		return 0, ErrConflict
	}
	n, err := w.destination.Write(contents)
	w.remaining -= int64(n)
	return n, err
}

type downloadRange struct {
	offset int64
	size   int64
}

// copyDownloadRanges reads non-overlapping ranges over independent SFTP
// connections. The local spool still becomes the single immutable, hashed
// representation used by HTTP retries and browser checkpoints.
func (s Service) copyDownloadRanges(
	ctx context.Context, firstRemote Remote, alias, remotePath string, destination *os.File, size int64, parallelism int, chunkBytes int64,
	progress func(DownloadPartProgress),
) (int64, error) {
	if size <= 0 || parallelism <= 1 || chunkBytes <= 0 {
		return 0, ErrInvalidTransfer
	}
	ranges := make([]downloadRange, 0, int((size+chunkBytes-1)/chunkBytes))
	for offset := int64(0); offset < size; offset += chunkBytes {
		ranges = append(ranges, downloadRange{offset: offset, size: min(chunkBytes, size-offset)})
	}
	if err := destination.Truncate(size); err != nil {
		return 0, err
	}
	workerContext, cancel := context.WithCancel(ctx)
	defer cancel()
	workerCount := min(parallelism, len(ranges))
	assignments := make([][]downloadRange, workerCount)
	for index, portion := range ranges {
		worker := index % workerCount
		assignments[worker] = append(assignments[worker], portion)
	}
	writtenParts := make(chan int64, len(ranges))
	var workers sync.WaitGroup
	var firstErr error
	var errorOnce sync.Once
	fail := func(err error) {
		errorOnce.Do(func() {
			firstErr = err
			cancel()
		})
	}
	for workerIndex := range workerCount {
		var workerTotal int64
		for _, portion := range assignments[workerIndex] {
			workerTotal += portion.size
		}
		if progress != nil {
			progress(DownloadPartProgress{Index: workerIndex, TotalBytes: workerTotal})
		}
		workers.Add(1)
		go func(workerIndex int, workerTotal int64) {
			defer workers.Done()
			remote := firstRemote
			if workerIndex != 0 {
				var err error
				remote, err = s.openRequest(workerContext, alias)
				if err != nil {
					fail(err)
					return
				}
				defer remote.Close()
			}
			ranged, ok := remote.(RangeRemote)
			if !ok {
				fail(ErrInvalidTransfer)
				return
			}
			var workerWritten int64
			for _, portion := range assignments[workerIndex] {
				last := portion.offset+portion.size == size
				written, err := copyDownloadRange(workerContext, ranged, remotePath, destination, portion, last, func(chunkWritten int64) {
					if progress != nil {
						progress(DownloadPartProgress{Index: workerIndex, TransferredBytes: workerWritten + chunkWritten, TotalBytes: workerTotal})
					}
				})
				if err != nil {
					fail(err)
					return
				}
				workerWritten += written
				writtenParts <- written
			}
		}(workerIndex, workerTotal)
	}
	workers.Wait()
	close(writtenParts)
	if firstErr != nil {
		return 0, firstErr
	}
	var written int64
	for part := range writtenParts {
		written += part
	}
	return written, nil
}

func copyDownloadRange(
	ctx context.Context, remote RangeRemote, remotePath string, destination *os.File, portion downloadRange, last bool,
	progress func(int64),
) (int64, error) {
	source, err := remote.OpenRange(remotePath, portion.offset)
	if err != nil {
		return 0, err
	}
	defer source.Close()
	output := io.Writer(io.NewOffsetWriter(destination, portion.offset))
	if progress != nil {
		output = &downloadProgressWriter{destination: output, progress: progress}
	}
	written, err := copyContext(ctx, output, io.LimitReader(source, portion.size), 0)
	if err != nil {
		return written, err
	}
	if written != portion.size {
		return written, ErrConflict
	}
	if last {
		var extra [1]byte
		if count, readErr := source.Read(extra[:]); count != 0 || (readErr != nil && !errors.Is(readErr, io.EOF)) {
			return written, ErrConflict
		}
	}
	return written, nil
}

type downloadProgressWriter struct {
	destination io.Writer
	written     int64
	progress    func(int64)
}

func (writer *downloadProgressWriter) Write(contents []byte) (int, error) {
	written, err := writer.destination.Write(contents)
	writer.written += int64(written)
	writer.progress(writer.written)
	return written, err
}
