package sftp

import (
	"context"
	"errors"
	"io/fs"
	"path"
)

// cleanupServerFilePart removes only the deterministic unpublished sibling.
// A possibly completed target is never addressed by cancellation or removal.
func (m *TransferManager) cleanupServerFilePart(job TransferJob) (operationErr error) {
	if !recoverableRemoteFile(job) || job.RemoteCheckpoint == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), uploadPartCleanupTimeout)
	defer cancel()
	end := &serverFileEnd{ctx: ctx, service: *m.Service, name: job.RemotePath}
	if job.Operation == RemoteGet {
		root, relative, err := openLocalRoot(job.RemotePath)
		if err != nil {
			return labelLocalAccessRefusal(err)
		}
		end.root = root
		end.part = path.Join(path.Dir(relative), ".sshc-download-"+job.ID)
	} else {
		remote, err := m.Service.openRequest(ctx, job.Alias)
		if err != nil {
			return err
		}
		end.remote = remote
		end.part = uploadPartPath(job.RemotePath, job.ID)
	}
	defer func() { end.close(operationErr) }()
	if _, err := end.stat(end.part); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if end.root != nil {
		return labelLocalAccessRefusal(end.root.Remove(end.part))
	}
	return end.remote.Remove(end.part)
}

func (m *TransferManager) cleanupCancelledServerFile(id string) {
	m.jobsMutex.Lock()
	record := m.jobs[id]
	if record == nil || record.job.Status != TransferCancelled || record.job.RemoteCheckpoint == nil {
		m.jobsMutex.Unlock()
		return
	}
	job := record.job
	m.jobsMutex.Unlock()
	err := m.cleanupServerFilePart(job)
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	record = m.jobs[id]
	if record == nil || record.job.Status != TransferCancelled {
		return
	}
	original := cloneTransferJobRecord(record)
	if err == nil {
		record.job.RemoteCheckpoint = nil
	} else {
		record.job.Problem = CleanupPendingProblem
	}
	if err := m.persistJobsLocked(true); err != nil {
		*record = original
	}
}

func (m *TransferManager) removeCancelledServerFiles() error {
	m.jobsMutex.Lock()
	var cancelled []string
	for id, record := range m.jobs {
		if record.job.Status == TransferCancelled && record.job.RemoteCheckpoint != nil && m.dataPlane[id] == 0 {
			cancelled = append(cancelled, id)
		}
	}
	m.jobsMutex.Unlock()
	for _, id := range cancelled {
		if err := m.RemoveJob(id); err != nil {
			return err
		}
	}
	return nil
}
