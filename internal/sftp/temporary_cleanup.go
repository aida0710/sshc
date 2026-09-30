package sftp

import (
	"context"
	"errors"
	"io/fs"
	"time"
)

// temporaryCleanupTimeout bounds the removal of an unpublished temporary
// through a new connection. It runs right after a pause or cancel, and a host
// that does not answer within it would hold the worker; what it leaves behind
// is removed later as an abandoned temporary.
const temporaryCleanupTimeout = 30 * time.Second

// abandonedTemporaryAge is how long a temporary must go unmodified before a
// delete or a move treats it as left behind instead of as a write in
// progress. Every writer (put, remote copy, editor save) keeps writing its
// temporary and removes it when it stops, and none sets another time on it:
// put and remote copy give the source's time to the target after publishing.
// A day without a write therefore cannot be a live one; the margin also
// covers clock skew between engine and server.
const abandonedTemporaryAge = 24 * time.Hour

// unpublishedFile is a temporary that a transfer writes beside its target
// before publishing it by rename.
type unpublishedFile struct {
	service Service
	// alias opens another connection to the host holding the file.
	alias  string
	remote Remote
	path   string
}

// remove deletes the temporary after the transfer failed or stopped. A
// transfer stopped by pause or cancel has lost its connection already:
// cancelling the context discards it (see bindRemoteContext). The removal is
// then retried once through a new connection that the cancellation does not
// reach. A host that allows one connection (one-time code) may refuse it;
// the file is then left for isAbandonedTemporary.
func (f unpublishedFile) remove(ctx context.Context) error {
	err := f.remote.Remove(f.path)
	if err == nil || errors.Is(err, fs.ErrNotExist) || ctx.Err() == nil {
		return ignoreNotExist(err)
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), temporaryCleanupTimeout)
	defer cancel()
	fresh, openErr := f.service.openRequest(cleanupCtx, f.alias)
	if openErr != nil {
		return errors.Join(err, openErr)
	}
	defer fresh.Close()
	return ignoreNotExist(fresh.Remove(f.path))
}

func ignoreNotExist(err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// isAbandonedTemporary reports whether info is a temporary of a put, remote
// copy or editor save that nobody has written for abandonedTemporaryAge, such
// as one whose connection was cut before it could be removed. Upload parts
// are never abandoned here: a paused resumable upload keeps its part on
// purpose, and its job removes it.
func isAbandonedTemporary(info fs.FileInfo, now time.Time) bool {
	return editorTemporaryNamePattern.MatchString(info.Name()) && info.Mode().IsRegular() &&
		now.Sub(info.ModTime()) > abandonedTemporaryAge
}
