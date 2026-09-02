package sftp

import (
	"context"
	"errors"
	"time"
)

// ScheduleRemoteJob starts an engine-owned Remote-to-Remote transfer. Repeated
// calls are harmless; only one worker may own a job at a time.
func (m *TransferManager) ScheduleRemoteJob(id string) {
	if m == nil || m.Service == nil || !transferIDPattern.MatchString(id) || m.isClosed() {
		return
	}
	m.remoteJobsMutex.Lock()
	if _, running := m.remoteCancels[id]; running {
		m.remoteJobsMutex.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.remoteCancels[id] = cancel
	m.remoteJobsMutex.Unlock()
	go m.runRemoteJob(ctx, id)
}

func (m *TransferManager) cancelRemoteJob(id string) {
	m.remoteJobsMutex.Lock()
	cancel := m.remoteCancels[id]
	m.remoteJobsMutex.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (m *TransferManager) finishRemoteWorker(id string) {
	m.remoteJobsMutex.Lock()
	if cancel := m.remoteCancels[id]; cancel != nil {
		cancel()
		delete(m.remoteCancels, id)
	}
	m.remoteJobsMutex.Unlock()
}

func (m *TransferManager) runRemoteJob(ctx context.Context, id string) {
	defer m.finishRemoteWorker(id)
	var job TransferJob
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		started, err := m.UpdateJob(id, UpdateTransferJob{Action: TransferStartAction})
		if err == nil {
			job = started
			break
		}
		if !errors.Is(err, ErrTransferLimit) {
			return
		}
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
	plan, err := m.Service.PlanRemoteTransfer(ctx, RemoteTransferRequest{
		SourceAlias: job.SourceAlias, SourcePath: job.SourcePath,
		TargetAlias: job.Alias, TargetPath: job.RemotePath,
		Operation: job.Operation, Overwrite: job.Overwrite,
	})
	if err == nil {
		zero := int64(0)
		total := plan.TotalBytes
		_, err = m.UpdateJob(id, UpdateTransferJob{Action: TransferProgressAction, TransferredBytes: &zero, TotalBytes: &total, ResetProgress: true})
	}
	if err == nil {
		err = m.Service.CopyRemote(ctx, RemoteTransferRequest{
			SourceAlias: job.SourceAlias, SourcePath: job.SourcePath,
			TargetAlias: job.Alias, TargetPath: job.RemotePath,
			Operation: job.Operation, Overwrite: job.Overwrite,
		}, func(transferred int64) error {
			_, progressErr := m.UpdateJob(id, UpdateTransferJob{Action: TransferProgressAction, TransferredBytes: &transferred})
			return progressErr
		})
	}
	if err == nil {
		completed := plan.TotalBytes
		_, _ = m.UpdateJob(id, UpdateTransferJob{Action: TransferCompleteAction, TransferredBytes: &completed})
		return
	}
	if errors.Is(err, context.Canceled) {
		return
	}
	if errors.Is(err, ErrAlreadyExists) {
		_, _ = m.UpdateJob(id, UpdateTransferJob{Action: TransferNeedsOverwriteAction})
		return
	}
	_, _ = m.UpdateJob(id, UpdateTransferJob{Action: TransferFailAction, Problem: remoteTransferProblem(err)})
}

func remoteTransferProblem(err error) string {
	switch {
	case errors.Is(err, ErrConflict):
		return "sftp_conflict"
	case errors.Is(err, ErrUnsupportedEntry):
		return "sftp_unsupported_entry"
	case errors.Is(err, ErrCompareLimit), errors.Is(err, ErrTransferTooLarge):
		return "sftp_transfer_too_large"
	default:
		return "sftp_failed"
	}
}
