package sftp

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"time"
)

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
