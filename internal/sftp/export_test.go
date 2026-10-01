package sftp

import (
	"context"
	"time"
)

// PrepareDownloadForTest spools one remote file sequentially into the OS
// temporary directory. Production downloads go through
// TransferManager.PrepareOwnedDownload, which needs a running job; tests that
// only check the spool itself use this instead of creating one.
func (s Service) PrepareDownloadForTest(ctx context.Context, alias, remotePath string) (*PreparedDownload, error) {
	return s.prepareDownload(ctx, DownloadRequest{Alias: alias, RemotePath: remotePath, SplitParallelism: 1})
}

// DeleteTreeForTest removes remotePath and everything below it without
// comparing against a planned count. Production deletes run as a job, which
// passes the count PlanDelete found so that a tree changed in between is
// refused; tests that only need a tree gone use this instead.
func (s Service) DeleteTreeForTest(ctx context.Context, alias, remotePath string) error {
	return s.DeleteWithProgress(ctx, alias, remotePath, -1, nil)
}

// ScheduleIdleRemoteDiscardsForTest replaces how the manager schedules the
// discard of a connection a sequential upload keeps between chunks, so that a
// test fires the discard instead of waiting for transferRemoteIdleTimeout.
func (m *TransferManager) ScheduleIdleRemoteDiscardsForTest(after func(time.Duration, func()) *time.Timer) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.after = after
}

// StartUploadPartForTest, AppendUploadPartForTest, CompleteUploadPartForTest
// and CancelUploadPartForTest work on an upload part without a job. Production
// uploads go through StartOwned, AppendOwned, CompleteOwned and CancelOwned,
// which check the job's state, owner lock and queue slot first; tests of the
// part itself use these instead of creating and starting a job.
//
// StartUploadPartForTest prepares a part written as one stream, and resumes
// after every byte the part already holds.
func (m *TransferManager) StartUploadPartForTest(ctx context.Context, alias, id, remotePath string, options StartUploadOptions) (ResumableUpload, error) {
	return m.startSequentialUpload(ctx, UploadTarget{Alias: alias, ID: id, RemotePath: remotePath}, options, options.Size)
}

func (m *TransferManager) AppendUploadPartForTest(ctx context.Context, chunk UploadAppend) (ResumableUpload, error) {
	return m.appendUploadPart(ctx, chunk)
}

func (m *TransferManager) CompleteUploadPartForTest(ctx context.Context, completion UploadCompletion) (Transfer, error) {
	return m.completeUploadPart(ctx, completion)
}

func (m *TransferManager) CancelUploadPartForTest(ctx context.Context, alias, id, remotePath string) error {
	return m.cancelUploadPart(ctx, alias, id, remotePath)
}

// MaxDeleteDepthForTest, MaxTransferTreeEntriesForTest, MaxSearchDepthForTest
// and MaxSearchVisitedForTest let a test build a tree just beyond what a
// delete, a transfer or the bounded walk of search, stats and chmod reaches.
const (
	MaxDeleteDepthForTest         = maxDeleteDepth
	MaxTransferTreeEntriesForTest = maxTransferTreeEntries
	MaxSearchDepthForTest         = maxSearchDepth
	MaxSearchVisitedForTest       = maxSearchVisited
)

// SymlinkForTest creates link on the server, pointing at target. sshc never
// creates links itself; integration tests use it to build a path that reaches
// back into a folder through a link.
func (c *Client) SymlinkForTest(target, link string) error {
	return c.client.Symlink(target, link)
}
