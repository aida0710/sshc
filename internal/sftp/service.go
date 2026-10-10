package sftp

import (
	"context"
	"io"
	"io/fs"
	"path"
	"strings"
	"sync"

	"sshc/internal/validate"
)

type Service struct {
	// Installed once at engine startup; copies of Service share this budget.
	transferLimiter    *transferLimiter
	transferExclusions func() []string
	// Non-nil selects a job snapshot, including an explicitly empty one.
	selectedExclusions []string
	Open               OpenRemote
	// TemporaryPath はテスト時に差し替える。本番では対象と同じディレクトリへ予測不能な名前を作る。
	TemporaryPath func(target string) (string, error)
	// ConnectionLimit says how many connections one transfer may open to a
	// host at once, or 0 for no limit. A host that authenticates with a
	// one-time code allows one: a second connection in the same window
	// presents a code the server has already accepted, and is refused. Split
	// downloads and split uploads bound their ranges by it, and a copy within
	// one host shares one connection. The pool does not enforce it, so
	// separate operations running at once may still open more.
	ConnectionLimit func(alias string) int
}

// boundedParallelism trims the parallel connections a transfer asked for to
// what the host allows.
func (s Service) boundedParallelism(alias string, requested int) int {
	if s.ConnectionLimit == nil {
		return requested
	}
	if limit := s.ConnectionLimit(alias); limit > 0 && limit < requested {
		return limit
	}
	return requested
}

const (
	previewSniffBytes = 512

	// 検索は「速く終わる」ことを機能の一部として扱う。SFTP の往復は高く、
	// 深い木を全部歩けば数分かかる。予算に当たったら、そこまでの一致と
	// 「まだ続きがある」を返して終わる。
	MaxSearchQueryBytes = 128
	maxSearchResults    = 200
	maxSearchVisited    = 20_000
	maxSearchDepth      = 32

	maxArchiveEntries = 10_000
	maxArchiveDepth   = 64
	maxArchiveBytes   = int64(1 << 30)
	// A regular file transfer is explicitly requested and may be much larger
	// than an archive assembled from an entire directory. Keep a finite safety
	// bound, but do not reuse the 1 GiB archive-expansion budget for files.
	maxRegularFileTransferBytes = int64(512 << 30)
)

func (s Service) open(ctx context.Context, alias string) (Remote, error) {
	if err := validateAlias(alias); err != nil {
		return nil, err
	}
	if s.Open == nil {
		return nil, ErrUnavailable
	}
	return s.Open(ctx, alias)
}

// openRequest binds an operation-scoped remote to the request context. Closing
// the SFTP transport is the only portable way to wake a pkg/sftp Read or Write
// which is already blocked when its context is cancelled.
func (s Service) openRequest(ctx context.Context, alias string) (Remote, error) {
	remote, err := s.open(ctx, alias)
	if err != nil {
		return nil, err
	}
	return bindRemoteContext(ctx, remote), nil
}

func cleanPath(candidate string, allowRoot bool) (string, error) {
	if candidate == "" || strings.IndexByte(candidate, 0) >= 0 || !path.IsAbs(candidate) {
		return "", ErrInvalidPath
	}
	cleaned := path.Clean(candidate)
	if !allowRoot && cleaned == "/" {
		return "", ErrRootOperation
	}
	return cleaned, nil
}

func cleanPublicPath(candidate string, allowRoot bool) (string, error) {
	cleaned, err := cleanPath(candidate, allowRoot)
	if err != nil {
		return "", err
	}
	for _, segment := range strings.Split(strings.TrimPrefix(cleaned, "/"), "/") {
		if isInternalName(segment) {
			return "", ErrInvalidPath
		}
	}
	return cleaned, nil
}

func validateAlias(alias string) error {
	if strings.TrimSpace(alias) == "" {
		return ErrInvalidAlias
	}
	return validate.Alias(alias)
}

type contextRemote struct {
	Remote
	ctx      context.Context
	once     sync.Once
	mutex    sync.Mutex
	stop     func() bool
	closed   bool
	closeErr error
}

func bindRemoteContext(ctx context.Context, remote Remote) Remote {
	bound := &contextRemote{Remote: remote, ctx: ctx}
	stop := context.AfterFunc(ctx, func() { _ = bound.discard() })
	bound.mutex.Lock()
	bound.stop = stop
	closed := bound.closed
	bound.mutex.Unlock()
	if closed {
		stop()
	}
	if _, ok := remote.(RangeRemote); ok {
		return &contextRangeRemote{contextRemote: bound}
	}
	return bound
}

type contextRangeRemote struct{ *contextRemote }

func (remote *contextRangeRemote) OpenRange(candidate string, offset int64) (io.ReadCloser, error) {
	ranged, ok := remote.Remote.(RangeRemote)
	if !ok {
		return nil, ErrInvalidTransfer
	}
	return ranged.OpenRange(candidate, offset)
}

func (remote *contextRemote) Discard() error { return remote.finish(true) }

func (remote *contextRemote) Close() error {
	return remote.finish(false)
}

// discard is the cancellation path. A pooled connection must not go back to
// the pool with a request possibly still in flight on it.
func (remote *contextRemote) discard() error {
	return remote.finish(true)
}

func (remote *contextRemote) finish(cancelled bool) error {
	remote.once.Do(func() {
		remote.mutex.Lock()
		remote.closed = true
		stop := remote.stop
		remote.mutex.Unlock()
		if stop != nil {
			stop()
		}
		// The AfterFunc and the operation's own Close race once the context is
		// cancelled; either way the connection is not returned.
		if discardable, ok := remote.Remote.(discardableRemote); ok && (cancelled || remote.ctx.Err() != nil) {
			remote.closeErr = discardable.Discard()
			return
		}
		remote.closeErr = remote.Remote.Close()
	})
	return remote.closeErr
}

func entryTypeOf(info fs.FileInfo) EntryType {
	if !metadataTypeKnown(info) {
		return EntryOther
	}
	switch {
	case info.IsDir():
		return EntryDirectory
	case info.Mode().IsRegular():
		return EntryFile
	case info.Mode()&fs.ModeSymlink != 0:
		return EntrySymlink
	}
	return EntryOther
}

func entryFrom(parent string, info fs.FileInfo) Entry {
	entry := Entry{
		Name:       info.Name(),
		Path:       path.Join(parent, info.Name()),
		Type:       entryTypeOf(info),
		Size:       info.Size(),
		Mode:       info.Mode(),
		ModifiedAt: info.ModTime().UTC(),
		Revision:   metadataRevision(info),
	}
	if owner, available := ownershipFrom(info); available {
		entry.Ownership = &owner
	}
	return entry
}

// copyChunkBytes is how much one read or write of a copy asks for. pkg/sftp
// splits a larger request into 32 KiB packets and keeps up to 64 of them in
// flight on one file, so 2 MiB fills that pipeline. With io.Copy's 32 KiB
// buffer every packet waits a full round trip.
const copyChunkBytes = 2 << 20

var copyChunks = sync.Pool{New: func() any {
	chunk := make([]byte, copyChunkBytes)
	return &chunk
}}

func copyContext(ctx context.Context, destination io.Writer, source io.Reader, maxBytes int64) (int64, error) {
	reader := io.Reader(&contextReader{ctx: ctx, reader: source})
	if maxBytes > 0 {
		reader = io.LimitReader(reader, maxBytes+1)
	}
	chunk := copyChunks.Get().(*[]byte)
	defer copyChunks.Put(chunk)
	written, err := copyInChunks(destination, reader, *chunk)
	if err != nil {
		return written, err
	}
	if maxBytes > 0 && written > maxBytes {
		return written, ErrTransferTooLarge
	}
	return written, nil
}

// copyInChunks fills chunk before each write, so a source that returns a few
// KiB at a time, such as an HTTP body, still reaches the SFTP file as large
// writes. It is io.CopyBuffer without the WriterTo and ReaderFrom shortcuts:
// *os.File.ReadFrom copies any other reader through a 32 KiB buffer.
func copyInChunks(destination io.Writer, source io.Reader, chunk []byte) (int64, error) {
	var written int64
	for {
		filled, readErr := fillChunk(source, chunk)
		if filled > 0 {
			count, writeErr := destination.Write(chunk[:filled])
			written += int64(count)
			if writeErr != nil {
				return written, writeErr
			}
			if count != filled {
				return written, io.ErrShortWrite
			}
		}
		if readErr == io.EOF {
			return written, nil
		}
		if readErr != nil {
			return written, readErr
		}
	}
}

func fillChunk(source io.Reader, chunk []byte) (int, error) {
	filled := 0
	for filled < len(chunk) {
		read, err := source.Read(chunk[filled:])
		filled += read
		if err != nil {
			return filled, err
		}
	}
	return filled, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(contents []byte) (int, error) {
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
		read, err := r.reader.Read(contents)
		if err != nil && r.ctx.Err() != nil {
			return read, r.ctx.Err()
		}
		return read, err
	}
}

type namedInfo struct {
	fs.FileInfo
	name string
}

func (i namedInfo) Name() string                    { return i.name }
func (info namedInfo) Ownership() (Ownership, bool) { return ownershipFrom(info.FileInfo) }

type closedWriter struct{}

func (closedWriter) Write([]byte) (int, error) { return 0, fs.ErrClosed }
func (closedWriter) Close() error              { return nil }
