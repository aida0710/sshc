package sftp

// If a publication cannot be made durable, keep the in-memory queue blocked
// from repeat operations too. The last durable remote intent remains the
// authority on restart; an upload's original target revision also refuses a
// target that was already published.
func (m *TransferManager) markTransferReconciliationRequired(run *remoteRun, id string, transferred int64) {
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	record := m.jobs[id]
	if record == nil || (record.job.Direction != TransferRemote && record.job.Direction != TransferUpload) || record.job.Status != TransferRunning {
		return
	}
	if run.ownershipError() != nil {
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

func (m *TransferManager) recordUncertainUpload(id string) {
	if _, err := m.updateUploadJob(id, UpdateTransferJob{Action: TransferFailAction, Problem: RemoteReconciliationProblem}); err != nil {
		m.markTransferReconciliationRequired(nil, id, -1)
	}
}
