package sftp

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"time"
)

// serverFileEnd keeps the existing descriptor-rooted local access and SFTP
// access behind the same single-file operations. It checks for final symlinks,
// including when reopening an interrupted part, before opening the entry.
type serverFileEnd struct {
	ctx     context.Context
	service Service
	remote  Remote
	root    *os.Root
	name    string
	part    string
}

func (m *TransferManager) openServerFileEnds(ctx context.Context, job TransferJob) (*serverFileEnd, *serverFileEnd, error) {
	source := &serverFileEnd{ctx: ctx, service: *m.Service, name: job.SourcePath}
	target := &serverFileEnd{ctx: ctx, service: *m.Service, name: job.RemotePath}
	if job.Operation == RemotePut {
		root, relative, err := openLocalRoot(source.name)
		if err != nil {
			return nil, nil, labelLocalAccessRefusal(err)
		}
		source.root, source.name = root, relative
	} else {
		remote, err := m.Service.openRequest(ctx, job.SourceAlias)
		if err != nil {
			return nil, nil, err
		}
		source.remote = remote
	}
	if job.Operation == RemoteGet {
		root, relative, err := openLocalRoot(target.name)
		if err != nil {
			source.close(err)
			return nil, nil, labelLocalAccessRefusal(err)
		}
		target.root, target.name = root, relative
		target.part = path.Join(path.Dir(relative), ".sshc-download-"+job.ID)
		return source, target, nil
	}
	if job.Operation == RemoteCopy && job.SourceAlias == job.Alias {
		target.remote = source.remote
	} else {
		remote, err := m.Service.openRequest(ctx, job.Alias)
		if err != nil {
			source.close(err)
			return nil, nil, err
		}
		target.remote = remote
	}
	target.part = uploadPartPath(target.name, job.ID)
	return source, target, nil
}

func (end *serverFileEnd) close(operationErr error) {
	if end.root != nil {
		_ = end.root.Close()
		return
	}
	closeTransferRemote(end.remote, operationErr)
}

func (end *serverFileEnd) stat(name string) (fs.FileInfo, error) {
	if end.root == nil {
		info, err := end.remote.Lstat(name)
		if err == nil && !metadataContentKnown(info) {
			return nil, ErrMetadataUnavailable
		}
		if err == nil && !info.Mode().IsRegular() {
			return nil, ErrUnsupportedEntry
		}
		return info, err
	}
	info, err := checkLocal(end.root, name, true)
	if err != nil {
		return nil, labelLocalAccessRefusal(err)
	}
	if info == nil {
		return nil, fs.ErrNotExist
	}
	if !info.Mode().IsRegular() {
		return nil, ErrUnsupportedEntry
	}
	return info, nil
}

func (end *serverFileEnd) open(name string, offset int64) (io.ReadCloser, error) {
	if _, err := end.stat(name); err != nil {
		return nil, err
	}
	if end.root != nil {
		file, err := end.root.Open(name)
		if err != nil {
			return nil, labelLocalAccessRefusal(err)
		}
		if _, err := file.Seek(offset, io.SeekStart); err != nil {
			_ = file.Close()
			return nil, err
		}
		return file, nil
	}
	if offset > 0 {
		if remote, ok := end.remote.(RangeRemote); ok {
			return remote.OpenRange(name, offset)
		}
	}
	file, err := end.remote.Open(name)
	if err != nil {
		return nil, err
	}
	if offset > 0 {
		if _, err := io.CopyN(io.Discard, serverFileReader(end.ctx, end, file), offset); err != nil {
			_ = file.Close()
			return nil, err
		}
	}
	return file, nil
}

func (end *serverFileEnd) openPart(create bool) (WriteSeekCloser, error) {
	flags := os.O_WRONLY
	if create {
		flags |= os.O_CREATE | os.O_EXCL
	} else if _, err := end.stat(end.part); err != nil {
		return nil, err
	}
	if end.root == nil {
		return end.remote.OpenFile(end.part, flags)
	}
	// checkLocal rejects a symlink before Root.OpenFile can follow it.
	if _, err := checkLocal(end.root, end.part, true); err != nil {
		return nil, labelLocalAccessRefusal(err)
	}
	file, err := end.root.OpenFile(end.part, flags, 0o600)
	return file, labelLocalAccessRefusal(err)
}

func (end *serverFileEnd) revision() (string, error) {
	info, err := end.stat(end.name)
	if errors.Is(err, fs.ErrNotExist) {
		return AbsentRevision, nil
	}
	if err != nil {
		return "", err
	}
	digest, err := serverFileDigest(end.ctx, end)
	if err != nil {
		return "", err
	}
	after, err := end.stat(end.name)
	if err != nil {
		return "", err
	}
	if metadataRevision(info) != metadataRevision(after) {
		return "", ErrConflict
	}
	return metadataRevision(info) + ":" + digest, nil
}

func (end *serverFileEnd) publish(overwrite bool, modified time.Time) error {
	if end.root != nil {
		if err := end.root.Chtimes(end.part, modified, modified); err != nil {
			return labelLocalAccessRefusal(err)
		}
		if overwrite {
			return publicationFailure(labelLocalAccessRefusal(end.root.Rename(end.part, end.name)))
		}
		if err := publishLocalWithoutReplace(end.root, end.part, end.name); err != nil {
			return publicationFailure(labelLocalAccessRefusal(err))
		}
		// Hard-link publication leaves the old name; rename fallback does not.
		if err := end.root.Remove(end.part); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return errors.Join(ErrAmbiguousTransfer, labelLocalAccessRefusal(err))
		}
		return nil
	}
	var err error
	if overwrite {
		err = end.remote.Replace(end.part, end.name)
	} else {
		err = end.remote.Rename(end.part, end.name)
	}
	if err != nil {
		return publicationFailure(err)
	}
	// Keep a part's last-write time until publication so abandoned-part cleanup
	// cannot mistake a live copy of an old source for an abandoned transfer.
	if err := end.remote.Chtimes(end.name, modified); err != nil {
		return errors.Join(ErrAmbiguousTransfer, err)
	}
	return nil
}
