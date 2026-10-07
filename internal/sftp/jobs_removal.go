package sftp

import (
	"context"
	"errors"
	"time"
)

// uploadPartCleanupTimeout bounds how long removing an unpublished part
// holds up the request that needs the job gone: admitting a new job or
// removing the failed one. An unreachable host then fails that request instead
// of stalling it.
const uploadPartCleanupTimeout = 5 * time.Second

func (m *TransferManager) cleanupTransferPart(job TransferJob) error {
	m.releasePreparedDownload(job.ID)
	if recoverableRemoteFile(job) && job.RemoteCheckpoint != nil {
		return m.cleanupServerFilePart(job)
	}
	if job.Direction != TransferUpload || job.Kind != TransferFile || m.Service == nil || m.Service.Open == nil {
		return ErrTransferState
	}
	ctx, cancel := context.WithTimeout(context.Background(), uploadPartCleanupTimeout)
	defer cancel()
	return m.cancelUploadPart(ctx, job.Alias, job.ID, job.RemotePath)
}

// ClearFinished removes terminal records from the engine-owned ledger. Active,
// paused and failed work remains available for control or diagnosis.
func (m *TransferManager) ClearFinished() (int, error) {
	if err := m.removeCancelledServerFiles(); err != nil {
		return 0, err
	}
	return m.removeFinishedJobs(func(TransferJob, time.Time) bool { return true })
}

// removeFinishedJobs は、完了か取消で終わった記録のうち selected が選んだものを
// 台帳から外して保存し、外した数を返す。保存に失敗したら台帳を元に戻す。手動の
// 消去と期限切れの消去が同じ条件で消すよう、この関数だけが終わった記録を外す。
//
// data-plane の操作を持つ記録は、終わっていても外さない。応答を送っている途中の
// download や、報告を返す前の remote worker がまだ使っており、ここで外すと使用中の
// prepared download を閉じてしまう。操作が終われば次の消去で外れる。
//
// prepared download の解放は jobsMutex を手放してから行う。prepared download は
// 別の mutex が守っており、二つを重ねて取れば順序が閉じない。
func (m *TransferManager) removeFinishedJobs(selected func(job TransferJob, now time.Time) bool) (int, error) {
	m.jobsMutex.Lock()
	m.initializeJobsLocked()
	now := m.now().UTC()
	removing := make(map[string]bool)
	for _, id := range m.jobOrder {
		record := m.jobs[id]
		if record != nil && terminalTransferStatus(record.job.Status) && !mayHoldRemotePart(record.job) && m.dataPlane[id] == 0 && selected(record.job, now) {
			removing[id] = true
		}
	}
	if len(removing) == 0 {
		m.jobsMutex.Unlock()
		return 0, nil
	}
	snapshot := m.snapshotTransferQueueLocked()
	kept := make([]string, 0, len(m.jobOrder)-len(removing))
	for _, id := range m.jobOrder {
		if removing[id] {
			delete(m.jobs, id)
			continue
		}
		kept = append(kept, id)
	}
	m.jobOrder = kept
	if err := m.persistJobsLocked(true); err != nil {
		m.restoreTransferQueueLocked(snapshot)
		m.jobsMutex.Unlock()
		return 0, err
	}
	m.jobsMutex.Unlock()
	for id := range removing {
		m.releasePreparedDownload(id)
	}
	return len(removing), nil
}

// RemoveJob dismisses one retained transfer record. Failed regular uploads may
// still own an unpublished sibling on the remote server, so that exact part is
// removed before the ledger entry disappears. A missing record is already in
// the requested state and makes DELETE safe to retry after a lost response.
func (m *TransferManager) RemoveJob(id string) error {
	if m.isClosed() {
		return ErrUnavailable
	}
	if !transferIDPattern.MatchString(id) {
		return ErrInvalidTransfer
	}
	unlock := m.lock("", jobOwnerLockKey(id))
	defer unlock()

	m.jobsMutex.Lock()
	m.initializeJobsLocked()
	record := m.jobs[id]
	if record == nil {
		m.jobsMutex.Unlock()
		return nil
	}
	if !retainedTransferStatus(record.job.Status) || record.cleanupInFlight || m.dataPlane[id] != 0 {
		m.jobsMutex.Unlock()
		return ErrTransferState
	}
	job := record.job
	if mayHoldRemotePart(job) {
		record.cleanupInFlight = true
		if err := m.persistJobsLocked(true); err != nil {
			record.cleanupInFlight = false
			m.jobsMutex.Unlock()
			return err
		}
		m.jobsMutex.Unlock()

		cleanupErr := m.cleanupTransferPart(job)
		m.jobsMutex.Lock()
		current := m.jobs[id]
		if current == nil {
			m.jobsMutex.Unlock()
			return nil
		}
		current.cleanupInFlight = false
		if cleanupErr != nil {
			original := cloneTransferJobRecord(current)
			current.cleanupTombstone = true
			current.job.Problem = CleanupPendingProblem
			current.job.UpdatedAt = m.now().UTC()
			persistErr := m.persistJobsLocked(true)
			if persistErr != nil {
				*current = original
			}
			m.jobsMutex.Unlock()
			return errors.Join(cleanupErr, persistErr)
		}
	}
	orderIndex := m.jobOrderIndexLocked(id)
	removed := m.jobs[id]
	delete(m.jobs, id)
	delete(m.dataPlane, id)
	m.removeJobOrderLocked(id)
	if err := m.persistJobsLocked(true); err != nil {
		m.jobs[id] = removed
		m.insertJobOrderLocked(orderIndex, id)
		m.jobsMutex.Unlock()
		return err
	}
	m.jobsMutex.Unlock()
	m.releasePreparedDownload(id)
	return nil
}

// uploadJobHasRemotePart says whether the upload recorded preparing its part on
// the host: a target revision, an acknowledged byte or a completed range.
func uploadJobHasRemotePart(job TransferJob) bool {
	return job.Direction == TransferUpload && job.Kind == TransferFile &&
		(job.ExpectedRevision != "" || job.TransferredBytes > 0 || len(job.UploadRanges) > 0)
}

// mayHoldRemotePart requires cleanup before forgetting a failed upload or an
// interrupted server-file checkpoint. Completed jobs have published their
// part; browser upload cancellation already removes it (see CancelOwned).
func mayHoldRemotePart(job TransferJob) bool {
	return (job.Status == TransferFailed && uploadJobHasRemotePart(job)) ||
		((job.Status == TransferFailed || job.Status == TransferCancelled) && job.RemoteCheckpoint != nil)
}
