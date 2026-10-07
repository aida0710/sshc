package sftp

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"path"
)

// A handle stat checks what was opened, rather than trusting the path's stat.
// SFTP v3 exposes metadata, but does not expose inode identities or O_NOFOLLOW.
type statReadCloser interface {
	io.ReadCloser
	Stat() (fs.FileInfo, error)
}

type stableContentFile struct {
	reader statReadCloser
	size   int64
	verify func() error
}

type contentReadBudget struct {
	maxBytes  int64
	bytesRead int64
}

type contentStream struct {
	file        stableContentFile
	destination io.Writer
}

// These reads refer to an already listed regular file. Disappearance or a new
// link is a changed identity, rather than a fresh not-found/unsupported request.
func existingContentReadError(err error) error {
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, ErrUnsupportedEntry) {
		return ErrConflict
	}
	return err
}

type countedContentReader struct {
	io.Reader
	budget *contentReadBudget
}

func (reader countedContentReader) Read(buffer []byte) (int, error) {
	read, err := reader.Reader.Read(buffer)
	reader.budget.bytesRead += int64(read)
	return read, err
}

// Read exactly the observed size: even an unexpectedly growing source cannot
// make the operation read beyond its byte budget. Fstat detects size changes.
func (budget *contentReadBudget) stream(ctx context.Context, stream contentStream) error {
	if stream.file.size < 0 || stream.file.size > budget.maxBytes-budget.bytesRead {
		return ErrCompareLimit
	}
	reader := countedContentReader{Reader: io.LimitReader(&contextReader{ctx: ctx, reader: stream.file.reader}, stream.file.size), budget: budget}
	chunk := copyChunks.Get().(*[]byte)
	defer copyChunks.Put(chunk)
	read, err := copyInChunks(stream.destination, reader, *chunk)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if read != stream.file.size {
		return ErrConflict
	}
	verified := stream.file.verify()
	if err := ctx.Err(); err != nil {
		return err
	}
	return verified
}

// Every component is inspected with Lstat. This also refuses a directory
// replaced by a link between listing it and opening one of its children.
func inspectRemoteReadPath(remote Remote, candidate string) (map[string]fs.FileInfo, error) {
	metadata := make(map[string]fs.FileInfo)
	for current := candidate; ; current = path.Dir(current) {
		info, err := remote.Lstat(current)
		if err != nil {
			return nil, err
		}
		if !metadataTypeKnown(info) || info.Mode()&fs.ModeSymlink != 0 {
			return nil, ErrUnsupportedEntry
		}
		if current != candidate && !info.IsDir() {
			return nil, ErrConflict
		}
		metadata[current] = info
		if current == "/" {
			return metadata, nil
		}
	}
}

func verifyRemoteReadPath(remote Remote, metadata map[string]fs.FileInfo) error {
	for candidate, expected := range metadata {
		current, err := remote.Lstat(candidate)
		if err != nil || metadataRevision(current) != metadataRevision(expected) {
			return ErrConflict
		}
	}
	return nil
}

func readStableRemoteDirectory(ctx context.Context, remote Remote, directory string) ([]fs.FileInfo, error) {
	metadata, err := inspectRemoteReadPath(remote, directory)
	if err != nil {
		return nil, err
	}
	if !metadata[directory].IsDir() {
		return nil, ErrNotDirectory
	}
	children, err := readChildren(ctx, remote, directory)
	if err != nil {
		return nil, err
	}
	if err := verifyRemoteReadPath(remote, metadata); err != nil {
		return nil, err
	}
	return children, nil
}

func openStableRemoteFile(remote Remote, entry Entry) (stableContentFile, error) {
	metadata, err := inspectRemoteReadPath(remote, entry.Path)
	if err != nil {
		return stableContentFile{}, existingContentReadError(err)
	}
	expected := metadata[entry.Path]
	if !expected.Mode().IsRegular() || metadataRevision(expected) != entry.Revision {
		return stableContentFile{}, ErrConflict
	}
	opened, err := remote.Open(entry.Path)
	if err != nil {
		return stableContentFile{}, existingContentReadError(err)
	}
	reader, ok := opened.(statReadCloser)
	if !ok {
		opened.Close()
		return stableContentFile{}, ErrUnsupportedOperation
	}
	verify := func() error {
		current, err := reader.Stat()
		if err != nil || !metadataTypeKnown(current) || !current.Mode().IsRegular() || metadataRevision(current) != entry.Revision {
			return ErrConflict
		}
		return verifyRemoteReadPath(remote, metadata)
	}
	if err := verify(); err != nil {
		reader.Close()
		return stableContentFile{}, err
	}
	return stableContentFile{reader: reader, size: expected.Size(), verify: verify}, nil
}
