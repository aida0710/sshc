package sftp

import (
	"context"
	"errors"
	"time"
)

// ScheduleRemoteJob starts an engine-owned Remote-to-Remote transfer. Repeated
// calls are harmless; only one worker may own a job at a time.
func (m *TransferManager) ScheduleRemoteJob(id string) {
	if m == nil || m.Service == nil || !transferIDPattern.MatchString(id) {
		return
	}
	m.remoteJobsMutex.Lock()
	// Close marks the manager closed before it cancels workers. Rechecking under
	// the worker-registration lock prevents an Add racing with Close's Wait.
	if m.isClosed() {
		m.remoteJobsMutex.Unlock()
		return
	}
	if _, running := m.remoteCancels[id]; running {
		m.remoteJobsMutex.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.remoteCancels[id] = cancel
	m.remoteWorkers.Add(1)
	m.remoteJobsMutex.Unlock()
	go func() {
		defer m.remoteWorkers.Done()
		m.runRemoteJob(ctx, id)
	}()
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
	if err != nil {
		m.finishRemoteJobWithError(id, err, false)
		return
	}
	// Persist an explicit intent before the copy/move can publish target data or
	// remove a source. If the terminal queue commit later fails, restart restores
	// this job as reconciliation-required instead of automatically repeating it.
	if err = m.markRemoteCommitPending(id); err != nil {
		// The intent could not be recorded, so the copy has not started and no
		// external state changed. Report it like any other pre-transfer failure
		// instead of leaving a running row for the stale sweep to reap.
		m.finishRemoteJobWithError(id, err, false)
		return
	}
	err = m.Service.CopyRemote(ctx, RemoteTransferRequest{
		SourceAlias: job.SourceAlias, SourcePath: job.SourcePath,
		TargetAlias: job.Alias, TargetPath: job.RemotePath,
		Operation: job.Operation, Overwrite: job.Overwrite,
	}, func(transferred int64) error {
		_, progressErr := m.UpdateJob(id, UpdateTransferJob{Action: TransferProgressAction, TransferredBytes: &transferred})
		return progressErr
	})
	if err == nil {
		completed := plan.TotalBytes
		if _, commitErr := m.UpdateJob(id, UpdateTransferJob{Action: TransferCompleteAction, TransferredBytes: &completed}); commitErr != nil {
			m.markRemoteReconciliationRequired(id, completed)
		}
		return
	}
	if errors.Is(err, context.Canceled) {
		return
	}
	m.finishRemoteJobWithError(id, err, true)
}

func (m *TransferManager) markRemoteCommitPending(id string) error {
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	record := m.jobs[id]
	if record == nil {
		return ErrTransferNotFound
	}
	if record.job.Direction != TransferRemote || record.job.Status != TransferRunning {
		return ErrTransferState
	}
	original := cloneTransferJobRecord(record)
	record.job.Problem = RemoteReconciliationProblem
	record.job.UpdatedAt = m.now().UTC()
	if err := m.persistJobsLocked(true); err != nil {
		*record = original
		return err
	}
	return nil
}

func (m *TransferManager) markRemoteReconciliationRequired(id string, transferred int64) {
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	record := m.jobs[id]
	if record == nil || record.job.Direction != TransferRemote || record.job.Status != TransferRunning {
		return
	}
	m.releaseJobLocked(record.job.Status)
	record.job.Status = TransferReattach
	record.job.Problem = RemoteReconciliationProblem
	if transferred >= 0 && (record.job.TotalBytes < 0 || transferred <= record.job.TotalBytes) {
		record.job.TransferredBytes = transferred
	}
	record.job.BytesPerSecond = 0
	record.job.RemainingSeconds = -1
	record.job.UpdatedAt = m.now().UTC()
}

func (m *TransferManager) finishRemoteJobWithError(id string, err error, externalSideEffectsPossible bool) {
	var transitionErr error
	if errors.Is(err, ErrAlreadyExists) {
		_, transitionErr = m.UpdateJob(id, UpdateTransferJob{Action: TransferNeedsOverwriteAction})
	} else {
		_, transitionErr = m.UpdateJob(id, UpdateTransferJob{Action: TransferFailAction, Problem: remoteTransferProblem(err)})
	}
	if transitionErr != nil && externalSideEffectsPossible {
		m.markRemoteReconciliationRequired(id, -1)
	}
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
