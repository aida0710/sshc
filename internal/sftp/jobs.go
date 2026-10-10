package sftp

import "time"

type transferQueueSnapshot struct {
	jobs       map[string]*transferJobRecord
	jobOrder   []string
	activeJobs int
	dataPlane  map[string]int
}

// Queue callers receive independent slices; editing a returned job cannot
// change which entries a queued transfer will omit.
func cloneTransferJob(job TransferJob) TransferJob {
	if job.RemoteCheckpoint != nil {
		checkpoint := *job.RemoteCheckpoint
		job.RemoteCheckpoint = &checkpoint
	}
	job.ExcludePatterns = append([]string(nil), job.ExcludePatterns...)
	job.DownloadParts = append([]DownloadPartProgress(nil), job.DownloadParts...)
	job.UploadRanges = append([]UploadRange(nil), job.UploadRanges...)
	return job
}

func cloneTransferJobRecord(record *transferJobRecord) transferJobRecord {
	cloned := *record
	cloned.job = cloneTransferJob(record.job)
	return cloned
}

func (m *TransferManager) snapshotTransferQueueLocked() transferQueueSnapshot {
	snapshot := transferQueueSnapshot{
		jobs:       make(map[string]*transferJobRecord, len(m.jobs)),
		jobOrder:   append([]string(nil), m.jobOrder...),
		activeJobs: m.activeJobs,
		dataPlane:  make(map[string]int, len(m.dataPlane)),
	}
	for id, record := range m.jobs {
		cloned := cloneTransferJobRecord(record)
		snapshot.jobs[id] = &cloned
	}
	for id, active := range m.dataPlane {
		snapshot.dataPlane[id] = active
	}
	return snapshot
}

func (m *TransferManager) restoreTransferQueueLocked(snapshot transferQueueSnapshot) {
	m.jobs = snapshot.jobs
	m.jobOrder = snapshot.jobOrder
	m.activeJobs = snapshot.activeJobs
	m.dataPlane = snapshot.dataPlane
}

func (m *TransferManager) ListJobs() ([]TransferJob, error) {
	m.sweepPreparedDownloads()
	if err := m.expireFinishedJobs(); err != nil {
		return nil, err
	}
	m.jobsMutex.Lock()
	m.initializeJobsLocked()
	staleRemotes, err := m.sweepStaleJobsLocked()
	if err != nil {
		m.jobsMutex.Unlock()
		return nil, err
	}
	result := make([]TransferJob, 0, len(m.jobOrder))
	for _, id := range m.jobOrder {
		if record := m.jobs[id]; record != nil {
			job := cloneTransferJob(record.job)
			result = append(result, job)
		}
	}
	m.jobsMutex.Unlock()
	closeRemotes(staleRemotes)
	return result, nil
}

func (m *TransferManager) initializeJobsLocked() {
	if m.maxConcurrent <= 0 {
		m.maxConcurrent = DefaultTransferConcurrency
	}
	if m.now == nil {
		m.now = time.Now
	}
	if m.jobs == nil {
		m.jobs = make(map[string]*transferJobRecord)
	}
	if m.dataPlane == nil {
		m.dataPlane = make(map[string]int)
	}
}
