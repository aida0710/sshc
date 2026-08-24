package sftp

import (
	"path"
	"strings"
	"time"
)

const (
	DefaultTransferConcurrency = 2
	maxRetainedTransferJobs    = 200
	staleRunningTransferAfter  = 2 * time.Minute
)

type TransferDirection string

const (
	TransferUpload   TransferDirection = "upload"
	TransferDownload TransferDirection = "download"
)

type TransferKind string

const (
	TransferFile   TransferKind = "file"
	TransferFolder TransferKind = "folder"
)

type TransferJobStatus string

const (
	TransferQueued         TransferJobStatus = "queued"
	TransferRunning        TransferJobStatus = "running"
	TransferPaused         TransferJobStatus = "paused"
	TransferReattach       TransferJobStatus = "reattach"
	TransferNeedsOverwrite TransferJobStatus = "needs_overwrite"
	TransferCompleted      TransferJobStatus = "completed"
	TransferFailed         TransferJobStatus = "failed"
	TransferCancelled      TransferJobStatus = "cancelled"
)

type TransferJobAction string

const (
	TransferStartAction          TransferJobAction = "start"
	TransferPauseAction          TransferJobAction = "pause"
	TransferResumeAction         TransferJobAction = "resume"
	TransferRetryAction          TransferJobAction = "retry"
	TransferCancelAction         TransferJobAction = "cancel"
	TransferProgressAction       TransferJobAction = "progress"
	TransferCompleteAction       TransferJobAction = "complete"
	TransferFailAction           TransferJobAction = "fail"
	TransferNeedsOverwriteAction TransferJobAction = "needs_overwrite"
)

type TransferJob struct {
	ID               string
	BatchID          string
	Alias            string
	Direction        TransferDirection
	Kind             TransferKind
	Name             string
	RemotePath       string
	TotalBytes       int64
	TransferredBytes int64
	BytesPerSecond   float64
	RemainingSeconds int64
	Status           TransferJobStatus
	Attempt          int
	Problem          string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type CreateTransferJob struct {
	ID         string
	BatchID    string
	Alias      string
	Direction  TransferDirection
	Kind       TransferKind
	Name       string
	RemotePath string
	TotalBytes int64
}

type UpdateTransferJob struct {
	Action           TransferJobAction
	TransferredBytes *int64
	Problem          string
	ResetProgress    bool
}

type transferJobRecord struct {
	job         TransferJob
	sampleAt    time.Time
	sampleBytes int64
}

func (m *TransferManager) ConfigureJobs(maxConcurrent int, now func() time.Time) {
	if maxConcurrent <= 0 {
		maxConcurrent = DefaultTransferConcurrency
	}
	if now == nil {
		now = time.Now
	}
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.maxConcurrent = maxConcurrent
	m.now = now
	if m.jobs == nil {
		m.jobs = make(map[string]*transferJobRecord)
	}
}

func (m *TransferManager) MaxConcurrent() int {
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	return m.maxConcurrent
}

func (m *TransferManager) CreateJob(input CreateTransferJob) (TransferJob, error) {
	if !transferIDPattern.MatchString(input.ID) || !transferIDPattern.MatchString(input.BatchID) ||
		strings.TrimSpace(input.Alias) == "" || len(input.Alias) > 255 || input.TotalBytes < -1 ||
		(input.Direction != TransferUpload && input.Direction != TransferDownload) ||
		(input.Kind != TransferFile && input.Kind != TransferFolder) {
		return TransferJob{}, ErrInvalidTransfer
	}
	cleaned, err := cleanPath(input.RemotePath, false)
	if err != nil {
		return TransferJob{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = path.Base(cleaned)
	}
	if len(name) > 1024 {
		return TransferJob{}, ErrInvalidTransfer
	}

	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	m.sweepJobsLocked()
	if existing := m.jobs[input.ID]; existing != nil {
		if sameTransferIdentity(existing.job, input, cleaned, name) {
			return existing.job, nil
		}
		return TransferJob{}, ErrConflict
	}
	now := m.now().UTC()
	job := TransferJob{
		ID: input.ID, BatchID: input.BatchID, Alias: input.Alias, Direction: input.Direction,
		Kind: input.Kind, Name: name, RemotePath: cleaned, TotalBytes: input.TotalBytes,
		Status: TransferQueued, Attempt: 1, CreatedAt: now, UpdatedAt: now,
		RemainingSeconds: -1,
	}
	m.jobs[input.ID] = &transferJobRecord{job: job, sampleAt: now}
	m.jobOrder = append(m.jobOrder, input.ID)
	m.trimJobsLocked()
	return job, nil
}

func (m *TransferManager) ListJobs() []TransferJob {
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	m.sweepJobsLocked()
	result := make([]TransferJob, 0, len(m.jobOrder))
	for _, id := range m.jobOrder {
		if record := m.jobs[id]; record != nil {
			result = append(result, record.job)
		}
	}
	return result
}

func (m *TransferManager) UpdateJob(id string, update UpdateTransferJob) (TransferJob, error) {
	if !transferIDPattern.MatchString(id) {
		return TransferJob{}, ErrInvalidTransfer
	}
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	m.sweepJobsLocked()
	record := m.jobs[id]
	if record == nil {
		return TransferJob{}, ErrTransferNotFound
	}
	original := *record
	originalActive := m.activeJobs
	committed := false
	defer func() {
		if !committed {
			*record = original
			m.activeJobs = originalActive
		}
	}()
	job := &record.job
	now := m.now().UTC()
	if update.TransferredBytes != nil {
		transferred := *update.TransferredBytes
		if transferred < job.TransferredBytes || transferred < 0 || (job.TotalBytes >= 0 && transferred > job.TotalBytes) {
			return TransferJob{}, ErrOffsetMismatch
		}
		job.TransferredBytes = transferred
	}

	switch update.Action {
	case TransferStartAction:
		if job.Status != TransferQueued {
			return TransferJob{}, ErrTransferState
		}
		if m.activeJobs >= m.maxConcurrent {
			return TransferJob{}, ErrTransferLimit
		}
		job.Status = TransferRunning
		m.activeJobs++
		record.sampleAt, record.sampleBytes = now, job.TransferredBytes
	case TransferPauseAction:
		if job.Status != TransferQueued && job.Status != TransferRunning {
			return TransferJob{}, ErrTransferState
		}
		m.releaseJobLocked(job.Status)
		job.Status = TransferPaused
	case TransferResumeAction:
		if job.Status != TransferPaused && job.Status != TransferReattach && job.Status != TransferNeedsOverwrite {
			return TransferJob{}, ErrTransferState
		}
		if update.ResetProgress {
			job.TransferredBytes = 0
			job.BytesPerSecond, job.RemainingSeconds = 0, -1
			record.sampleAt, record.sampleBytes = now, 0
		}
		job.Status = TransferQueued
		job.Problem = ""
	case TransferRetryAction:
		if job.Status != TransferFailed {
			return TransferJob{}, ErrTransferState
		}
		if update.ResetProgress {
			job.TransferredBytes = 0
		}
		job.Status, job.Problem = TransferQueued, ""
		job.Attempt++
		job.BytesPerSecond, job.RemainingSeconds = 0, -1
		record.sampleAt, record.sampleBytes = now, job.TransferredBytes
	case TransferCancelAction:
		if terminalTransferStatus(job.Status) {
			if job.Status == TransferCancelled {
				committed = true
				return *job, nil
			}
			return TransferJob{}, ErrTransferState
		}
		m.releaseJobLocked(job.Status)
		job.Status = TransferCancelled
	case TransferProgressAction:
		if job.Status != TransferRunning || update.TransferredBytes == nil {
			return TransferJob{}, ErrTransferState
		}
		m.updateRateLocked(record, now)
	case TransferCompleteAction:
		if job.Status == TransferCompleted {
			committed = true
			return *job, nil
		}
		if job.Status != TransferRunning || (job.TotalBytes >= 0 && job.TransferredBytes != job.TotalBytes) {
			return TransferJob{}, ErrTransferState
		}
		m.releaseJobLocked(job.Status)
		job.Status, job.Problem = TransferCompleted, ""
		job.RemainingSeconds = 0
	case TransferFailAction:
		if job.Status != TransferRunning && job.Status != TransferQueued {
			return TransferJob{}, ErrTransferState
		}
		m.releaseJobLocked(job.Status)
		job.Status, job.Problem = TransferFailed, boundedTransferProblem(update.Problem)
	case TransferNeedsOverwriteAction:
		if job.Status != TransferRunning && job.Status != TransferQueued {
			return TransferJob{}, ErrTransferState
		}
		m.releaseJobLocked(job.Status)
		job.Status, job.Problem = TransferNeedsOverwrite, "sftp_exists"
	default:
		return TransferJob{}, ErrInvalidTransfer
	}
	job.UpdatedAt = now
	committed = true
	return *job, nil
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
}

func (m *TransferManager) updateRateLocked(record *transferJobRecord, now time.Time) {
	elapsed := now.Sub(record.sampleAt).Seconds()
	if elapsed <= 0 {
		return
	}
	delta := record.job.TransferredBytes - record.sampleBytes
	if delta < 0 {
		return
	}
	instant := float64(delta) / elapsed
	if record.job.BytesPerSecond == 0 {
		record.job.BytesPerSecond = instant
	} else {
		record.job.BytesPerSecond = record.job.BytesPerSecond*0.65 + instant*0.35
	}
	if record.job.TotalBytes >= 0 && record.job.BytesPerSecond > 0 {
		remaining := float64(record.job.TotalBytes-record.job.TransferredBytes) / record.job.BytesPerSecond
		record.job.RemainingSeconds = int64(remaining + 0.5)
	} else {
		record.job.RemainingSeconds = -1
	}
	record.sampleAt, record.sampleBytes = now, record.job.TransferredBytes
}

func (m *TransferManager) sweepJobsLocked() {
	now := m.now().UTC()
	for _, record := range m.jobs {
		if record.job.Status == TransferRunning && now.Sub(record.job.UpdatedAt) > staleRunningTransferAfter {
			record.job.Status = TransferFailed
			record.job.Problem = "transfer_interrupted"
			record.job.UpdatedAt = now
			m.releaseJobLocked(TransferRunning)
		}
	}
}

func (m *TransferManager) trimJobsLocked() {
	for len(m.jobOrder) > maxRetainedTransferJobs {
		removed := false
		for index, id := range m.jobOrder {
			record := m.jobs[id]
			if record != nil && retainedTransferStatus(record.job.Status) {
				delete(m.jobs, id)
				m.jobOrder = append(m.jobOrder[:index], m.jobOrder[index+1:]...)
				removed = true
				break
			}
		}
		if !removed {
			break
		}
	}
}

func (m *TransferManager) releaseJobLocked(status TransferJobStatus) {
	if status == TransferRunning && m.activeJobs > 0 {
		m.activeJobs--
	}
}

func sameTransferIdentity(job TransferJob, input CreateTransferJob, cleaned, name string) bool {
	return job.BatchID == input.BatchID && job.Alias == input.Alias && job.Direction == input.Direction &&
		job.Kind == input.Kind && job.Name == name && job.RemotePath == cleaned && job.TotalBytes == input.TotalBytes
}

func terminalTransferStatus(status TransferJobStatus) bool {
	return status == TransferCompleted || status == TransferCancelled
}

func retainedTransferStatus(status TransferJobStatus) bool {
	return terminalTransferStatus(status) || status == TransferFailed
}

func boundedTransferProblem(problem string) string {
	problem = strings.TrimSpace(problem)
	if problem == "" {
		return "sftp_failed"
	}
	if len(problem) > 128 {
		return problem[:128]
	}
	return problem
}
