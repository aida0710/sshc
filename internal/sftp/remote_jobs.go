package sftp

import (
	"context"
	"errors"
)

// remoteRun is one worker's ownership of a remote job. Pause and cancel end the
// run by cancelling its context; resume and retry start a new run. A worker
// whose run has ended may still be blocked in a slow SFTP request, and once it
// returns the ledger refuses its reports (see transferUpdateRequest.run).
type remoteRun struct {
	ctx    context.Context
	cancel context.CancelFunc
}

// ownershipError is nil while the run still owns its job. A nil run is a
// report that does not come from a remote worker.
func (r *remoteRun) ownershipError() error {
	if r == nil {
		return nil
	}
	return r.ctx.Err()
}

func (r *remoteRun) end() {
	if r != nil {
		r.cancel()
	}
}

// scheduleRemoteJob starts an engine-owned remote operation. Repeated
// calls are harmless: a job keeps its live run, and only a run that pause,
// cancel or its own terminal report has ended is replaced by a new worker.
func (m *TransferManager) scheduleRemoteJob(id string) {
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
	if current := m.remoteRuns[id]; current != nil && current.ownershipError() == nil {
		m.remoteJobsMutex.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := &remoteRun{ctx: ctx, cancel: cancel}
	m.remoteRuns[id] = run
	m.remoteWorkers.Add(1)
	m.remoteJobsMutex.Unlock()
	go func() {
		defer m.remoteWorkers.Done()
		m.runRemoteJob(run, id)
	}()
}

// endRemoteRun ends the job's registered run, if any. The job itself keeps
// the state its caller gave it.
func (m *TransferManager) endRemoteRun(id string) {
	m.remoteJobsMutex.Lock()
	run := m.remoteRuns[id]
	m.remoteJobsMutex.Unlock()
	run.end()
}

// finishRemoteWorker unregisters run only while it is still the job's current
// run. A newer run started by resume keeps its registration.
func (m *TransferManager) finishRemoteWorker(id string, run *remoteRun) {
	m.remoteJobsMutex.Lock()
	run.end()
	if m.remoteRuns[id] == run {
		delete(m.remoteRuns, id)
	}
	delete(m.localTransferRuns, run)
	m.remoteJobsMutex.Unlock()
}

// reportRemoteRun applies a worker's report on its own run.
func (m *TransferManager) reportRemoteRun(run *remoteRun, id string, update UpdateTransferJob) (TransferJob, error) {
	if m.isClosed() {
		return TransferJob{}, ErrUnavailable
	}
	return m.updateJob(transferUpdateRequest{id: id, update: update, origin: transferUpdateInternal, run: run})
}

func (m *TransferManager) runRemoteJob(run *remoteRun, id string) {
	defer m.finishRemoteWorker(id, run)
	defer m.cleanupCancelledServerFile(id)
	ctx := run.ctx
	job, started := m.startRemoteJobWhenSlotFree(run, id)
	if !started {
		return
	}
	if err := m.protectLocalTransferRun(run, job); err != nil {
		return
	}
	// Planning and the operation itself can walk a large tree for longer than
	// the stale-running sweep tolerates without reporting progress. Holding the
	// data-plane count keeps the sweep from failing a job whose worker is alive.
	release, err := m.KeepJobActive(id)
	if err != nil {
		return
	}
	defer release()
	for {
		operation := remoteJobOperationFor(m.Service, job)
		totalBytes, err := operation.plan(ctx)
		if err == nil && job.RemoteCheckpoint == nil {
			zero := int64(0)
			job, err = m.reportRemoteRun(run, id, UpdateTransferJob{Action: TransferProgressAction, TransferredBytes: &zero, TotalBytes: &totalBytes, ResetProgress: true})
		}
		beforeOperation := err != nil
		if err == nil {
			// Persist intent before publication so an interrupted final queue
			// commit can never cause an automatic repeat after engine restart.
			err = m.markRemoteCommitPending(run, id)
			beforeOperation = err != nil
		}
		if err == nil {
			if recoverableRemoteFile(job) && (m.AutoReconnect() || job.RemoteCheckpoint != nil) {
				err = m.copyServerFile(run, job)
			} else {
				err = operation.run(ctx, totalBytes, func(transferred int64) error { return m.reportServerFileOffset(run, id, transferred) })
			}
		}
		if err == nil {
			completed := totalBytes
			if _, commitErr := m.reportRemoteRun(run, id, UpdateTransferJob{Action: TransferCompleteAction, TransferredBytes: &completed}); commitErr != nil {
				m.markTransferReconciliationRequired(run, id, completed)
			}
			return
		}
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			m.clearCommitPendingAfterPause(id)
			return
		}
		if recoverableRemoteFile(job) && ConnectionLost(err) && m.waitToReconnect(run, id) {
			m.jobsMutex.Lock()
			if record := m.jobs[id]; record != nil {
				job = record.job
			}
			m.jobsMutex.Unlock()
			continue
		}
		if beforeOperation {
			m.failRemoteJobBeforeOperation(run, id, err)
		} else {
			m.failRemoteJobAfterOperation(run, id, err)
		}
		return
	}
}

// startRemoteJobWhenSlotFree waits for a free transfer slot and marks the job
// running. It reports false when the run ended or the job cannot start for a
// reason other than a full queue.
func (m *TransferManager) startRemoteJobWhenSlotFree(run *remoteRun, id string) (TransferJob, bool) {
	for {
		if run.ctx.Err() != nil {
			return TransferJob{}, false
		}
		slotReleased, sweepAfter := m.slotWait()
		started, err := m.reportRemoteRun(run, id, UpdateTransferJob{Action: TransferStartAction})
		if err == nil {
			return started, true
		}
		if !errors.Is(err, ErrTransferLimit) || !waitForSlot(run.ctx, slotReleased, sweepAfter) {
			return TransferJob{}, false
		}
	}
}

// remoteJobOperation pairs how one kind of remote job is planned with how it
// runs. plan returns the total the run reports progress against, so that a
// new kind of job is added in one place.
type remoteJobOperation struct {
	plan func(ctx context.Context) (int64, error)
	run  func(ctx context.Context, totalBytes int64, report func(int64) error) error
}

func remoteJobOperationFor(service *Service, job TransferJob) remoteJobOperation {
	switch job.Operation {
	case RemoteDelete:
		return remoteJobOperation{
			plan: func(ctx context.Context) (int64, error) {
				return service.PlanDelete(ctx, job.Alias, job.RemotePath)
			},
			run: func(ctx context.Context, totalBytes int64, report func(int64) error) error {
				return service.DeleteWithProgress(ctx, job.Alias, job.RemotePath, totalBytes, report)
			},
		}
	case RemoteGet, RemotePut:
		request := remoteTransferRequestFor(job)
		return remoteJobOperation{
			plan: func(ctx context.Context) (int64, error) {
				plan, err := service.PlanLocalTransfer(ctx, request)
				return plan.TotalBytes, err
			},
			run: func(ctx context.Context, _ int64, report func(int64) error) error {
				return service.CopyLocal(ctx, request, report)
			},
		}
	default: // RemoteCopy and RemoteMove
		request := remoteTransferRequestFor(job)
		return remoteJobOperation{
			plan: func(ctx context.Context) (int64, error) {
				plan, err := service.PlanRemoteTransfer(ctx, request)
				return plan.TotalBytes, err
			},
			run: func(ctx context.Context, _ int64, report func(int64) error) error {
				return service.CopyRemote(ctx, request, report)
			},
		}
	}
}

// remoteTransferRequestFor names a get, put, copy or move job's source and
// target the way the Service operations take them. The job's alias and path
// are the target's.
func remoteTransferRequestFor(job TransferJob) RemoteTransferRequest {
	return RemoteTransferRequest{
		SourceAlias: job.SourceAlias, SourcePath: job.SourcePath,
		TargetAlias: job.Alias, TargetPath: job.RemotePath,
		Operation: job.Operation, Overwrite: job.Overwrite, ExcludePatterns: job.ExcludePatterns,
	}
}

func (m *TransferManager) markRemoteCommitPending(run *remoteRun, id string) error {
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	record := m.jobs[id]
	if record == nil {
		return ErrTransferNotFound
	}
	if err := run.ownershipError(); err != nil {
		return err
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

// clearCommitPendingAfterPause runs after a pause interrupted the operation
// with an error. The operation did not finish, so the job is in the same
// position as a failed one that retry runs again: resume re-plans the rest.
// Keeping the mark would leave a paused job that only cancel can clear.
func (m *TransferManager) clearCommitPendingAfterPause(id string) {
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	record := m.jobs[id]
	if record == nil || record.job.Direction != TransferRemote || record.job.Status != TransferPaused ||
		record.job.Problem != RemoteReconciliationProblem {
		return
	}
	original := cloneTransferJobRecord(record)
	record.job.Problem = ""
	record.job.UpdatedAt = m.now().UTC()
	if err := m.persistJobsLocked(true); err != nil {
		// Without a durable record the mark must stay; a restart would otherwise
		// restore the paused job still marked and the two views would disagree.
		*record = original
	}
}

// failRemoteJobBeforeOperation reports a failure from before the operation
// could change the remote side. A refused report leaves the job as it is:
// nothing happened that running it again would repeat.
func (m *TransferManager) failRemoteJobBeforeOperation(run *remoteRun, id string, err error) {
	_ = m.reportRemoteFailure(run, id, err)
}

// failRemoteJobAfterOperation reports a failure of the operation itself, which
// may already have published the target or removed the source. When that
// cannot be recorded, the job is marked for reconciliation so that it is never
// repeated automatically.
func (m *TransferManager) failRemoteJobAfterOperation(run *remoteRun, id string, err error) {
	if m.reportRemoteFailure(run, id, err) != nil {
		m.markTransferReconciliationRequired(run, id, -1)
	}
}

// reportRemoteFailure records how the run failed: an existing target asks for
// an overwrite, and anything else fails the job.
func (m *TransferManager) reportRemoteFailure(run *remoteRun, id string, err error) error {
	update := UpdateTransferJob{Action: TransferFailAction, Problem: remoteTransferProblem(err)}
	if errors.Is(err, ErrAlreadyExists) {
		update = UpdateTransferJob{Action: TransferNeedsOverwriteAction}
	}
	_, reportErr := m.reportRemoteRun(run, id, update)
	return reportErr
}

func remoteTransferProblem(err error) string {
	switch {
	case errors.Is(err, ErrAmbiguousTransfer):
		return RemoteReconciliationProblem
	case ConnectionLost(err):
		return "sftp_connection_lost"
	case errors.Is(err, ErrConflict):
		return "sftp_conflict"
	case errors.Is(err, ErrNameCollision):
		return "sftp_name_collision"
	case errors.Is(err, ErrUnsupportedEntry):
		return "sftp_unsupported_entry"
	case errors.Is(err, ErrTransferTooLarge):
		return "sftp_transfer_too_large"
	case errors.Is(err, ErrTraversalLimit):
		return "sftp_traversal_limit"
	case errors.Is(err, ErrTargetInsideSource):
		return "sftp_target_inside_source"
	case errors.Is(err, ErrTargetIsSource):
		return "sftp_target_is_source"
	case errors.Is(err, ErrLocalPrivacyProtection):
		return "sftp_local_privacy_protection"
	case errors.Is(err, ErrLocalPermissionDenied):
		return "sftp_local_permission_denied"
	default:
		return "sftp_failed"
	}
}
