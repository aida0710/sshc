package sftp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"regexp"
	"sync"
)

const AbsentRevision = "absent"

var transferIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`)
var uploadPartNamePattern = regexp.MustCompile(`^\..+\.sshc-upload-[A-Za-z0-9_-]{8,128}\.part$`)

// TransferManager serializes requests that operate on the same resumable part file.
// Different files remain independent so a large queue can make bounded parallel progress.
type TransferManager struct {
	Service *Service
	mutex   sync.Mutex
	locks   map[string]*transferLock
}

type transferLock struct {
	mutex sync.Mutex
	refs  int
}

func NewTransferManager(service *Service) *TransferManager {
	return &TransferManager{Service: service, locks: make(map[string]*transferLock)}
}

func (m *TransferManager) Start(ctx context.Context, alias, id, remotePath string, options StartUploadOptions) (ResumableUpload, error) {
	cleaned, err := resumablePath(id, remotePath)
	if err != nil || options.Size < 0 {
		return ResumableUpload{}, ErrInvalidTransfer
	}
	unlock := m.lock(alias, cleaned)
	defer unlock()
	remote, err := m.Service.open(ctx, alias)
	if err != nil {
		return ResumableUpload{}, err
	}
	defer remote.Close()

	expected, err := expectedTargetRevision(remote, cleaned, options)
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
	return ResumableUpload{ID: id, Path: cleaned, Offset: info.Size(), Size: options.Size, ExpectedRevision: expected}, nil
}

func (m *TransferManager) Append(ctx context.Context, alias, id, remotePath string, offset, total int64, contents []byte) (ResumableUpload, error) {
	cleaned, err := resumablePath(id, remotePath)
	if err != nil || offset < 0 || total < 0 || int64(len(contents)) > total-offset {
		return ResumableUpload{}, ErrOffsetMismatch
	}
	unlock := m.lock(alias, cleaned)
	defer unlock()
	remote, err := m.Service.open(ctx, alias)
	if err != nil {
		return ResumableUpload{}, err
	}
	defer remote.Close()
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
		written, err = io.Copy(file, bytes.NewReader(contents))
		if err == nil && written != int64(len(contents)) {
			err = io.ErrShortWrite
		}
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
	return ResumableUpload{ID: id, Path: cleaned, Offset: updated.Size(), Size: total}, nil
}

func (m *TransferManager) Complete(ctx context.Context, alias, id, remotePath string, total int64, expectedRevision string) (Transfer, error) {
	cleaned, err := resumablePath(id, remotePath)
	if err != nil || total < 0 || expectedRevision == "" {
		return Transfer{}, ErrInvalidTransfer
	}
	unlock := m.lock(alias, cleaned)
	defer unlock()
	remote, err := m.Service.open(ctx, alias)
	if err != nil {
		return Transfer{}, err
	}
	defer remote.Close()
	part := uploadPartPath(cleaned, id)
	info, err := remote.Lstat(part)
	if err != nil {
		return Transfer{}, err
	}
	if !info.Mode().IsRegular() || info.Size() != total {
		return Transfer{}, ErrUploadIncomplete
	}
	mode, err := verifyTargetRevision(remote, cleaned, expectedRevision)
	if err != nil {
		return Transfer{}, err
	}
	if err := remote.Chmod(part, mode); err != nil {
		return Transfer{}, err
	}
	if err := remote.Replace(part, cleaned); err != nil {
		return Transfer{}, err
	}
	updated, err := remote.Lstat(cleaned)
	if err != nil {
		return Transfer{}, err
	}
	return Transfer{Path: cleaned, Bytes: total, Revision: metadataRevision(updated)}, nil
}

func (m *TransferManager) Cancel(ctx context.Context, alias, id, remotePath string) error {
	cleaned, err := resumablePath(id, remotePath)
	if err != nil {
		return err
	}
	unlock := m.lock(alias, cleaned)
	defer unlock()
	remote, err := m.Service.open(ctx, alias)
	if err != nil {
		return err
	}
	defer remote.Close()
	err = remote.Remove(uploadPartPath(cleaned, id))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func expectedTargetRevision(remote Remote, target string, options StartUploadOptions) (string, error) {
	info, err := remote.Lstat(target)
	switch {
	case err == nil:
		if !info.Mode().IsRegular() {
			return "", ErrNotRegularFile
		}
		if !options.Overwrite {
			return "", ErrAlreadyExists
		}
		current := metadataRevision(info)
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

func verifyTargetRevision(remote Remote, target, expected string) (fs.FileMode, error) {
	info, err := remote.Lstat(target)
	if expected == AbsentRevision {
		if errors.Is(err, fs.ErrNotExist) {
			return 0o600, nil
		}
		if err != nil {
			return 0, err
		}
		return 0, ErrAlreadyExists
	}
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, ErrConflict
		}
		return 0, err
	}
	if !info.Mode().IsRegular() || metadataRevision(info) != expected {
		return 0, ErrConflict
	}
	return info.Mode().Perm(), nil
}

func resumablePath(id, remotePath string) (string, error) {
	if !transferIDPattern.MatchString(id) {
		return "", ErrInvalidTransfer
	}
	return cleanPath(remotePath, false)
}

func uploadPartPath(target, id string) string {
	return path.Join(path.Dir(target), "."+path.Base(target)+".sshc-upload-"+id+".part")
}

func isUploadPartName(name string) bool {
	return uploadPartNamePattern.MatchString(name)
}

func (m *TransferManager) lock(alias, target string) func() {
	key := alias + "\x00" + target
	m.mutex.Lock()
	entry := m.locks[key]
	if entry == nil {
		entry = &transferLock{}
		m.locks[key] = entry
	}
	entry.refs++
	m.mutex.Unlock()
	entry.mutex.Lock()
	return func() {
		entry.mutex.Unlock()
		m.mutex.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(m.locks, key)
		}
		m.mutex.Unlock()
	}
}
