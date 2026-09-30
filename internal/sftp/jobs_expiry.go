package sftp

import "time"

const staleRunningTransferAfter = 2 * time.Minute

// KeepJobActive protects an in-flight server-owned data operation from the
// stale-running sweep. It covers long remote hashing/spooling periods where no
// response bytes are available yet to report as progress.
func (m *TransferManager) KeepJobActive(id string) (func(), error) {
	if !transferIDPattern.MatchString(id) {
		return nil, ErrInvalidTransfer
	}
	m.jobsMutex.Lock()
	m.initializeJobsLocked()
	if m.jobs[id] == nil {
		m.jobsMutex.Unlock()
		return nil, ErrTransferNotFound
	}
	m.dataPlane[id]++
	m.jobsMutex.Unlock()
	return func() {
		m.jobsMutex.Lock()
		if m.dataPlane[id] <= 1 {
			delete(m.dataPlane, id)
		} else {
			m.dataPlane[id]--
		}
		m.jobsMutex.Unlock()
	}, nil
}

// staleTransferJob is a running job the sweep failed, and its record as it
// was before, so a failed save can put it back.
type staleTransferJob struct {
	record *transferJobRecord
	before transferJobRecord
}

// sweepStaleJobsLocked fails running jobs that stopped reporting and saves
// the queue. Only the swept records are kept for a failed save to restore:
// this runs on every job update, and copying the whole queue each time cost
// more than the update itself. The caller closes the returned connections
// after releasing jobsMutex.
func (m *TransferManager) sweepStaleJobsLocked() ([]Remote, error) {
	activeBefore := m.activeJobs
	stale := m.sweepJobsLocked()
	if len(stale) == 0 {
		return nil, nil
	}
	if err := m.persistJobsLocked(true); err != nil {
		for _, swept := range stale {
			*swept.record = swept.before
		}
		m.activeJobs = activeBefore
		return nil, err
	}
	return m.detachStaleRemotes(stale), nil
}

func (m *TransferManager) sweepJobsLocked() []staleTransferJob {
	now := m.now().UTC()
	var stale []staleTransferJob
	for _, record := range m.jobs {
		if m.dataPlane[record.job.ID] > 0 {
			continue
		}
		if record.job.Status == TransferRunning && now.Sub(record.job.UpdatedAt) > staleRunningTransferAfter {
			stale = append(stale, staleTransferJob{record: record, before: cloneTransferJobRecord(record)})
			record.job.Status = TransferFailed
			record.job.Problem = "transfer_interrupted"
			record.job.UpdatedAt = now
			m.releaseJobLocked(TransferRunning)
		}
	}
	return stale
}

func (m *TransferManager) detachStaleRemotes(stale []staleTransferJob) []Remote {
	detached := make([]Remote, 0, len(stale))
	for _, swept := range stale {
		job := swept.record.job
		if remote := m.detachRemote(job.Alias, job.ID, job.RemotePath); remote != nil {
			detached = append(detached, remote)
		}
	}
	return detached
}

// expireFinishedJobs は、完了と取消の記録を設定された時間で落とす。失敗は
// 残す。原因を見る前に消えてはならないからである。
func (m *TransferManager) expireFinishedJobs() error {
	_, err := m.removeFinishedJobs(func(job TransferJob, now time.Time) bool {
		return m.clearCompletedAfter > 0 && now.Sub(job.UpdatedAt) >= m.clearCompletedAfter
	})
	return err
}

func closeRemotes(remotes []Remote) {
	for _, remote := range remotes {
		discardRemote(remote)
	}
}
