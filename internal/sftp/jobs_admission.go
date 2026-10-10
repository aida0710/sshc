package sftp

import (
	"path"
	"strings"
)

const maxRetainedTransferJobs = 200

// maxTransferNameLength bounds the name and the batch name the transfer
// manager shows for a job. It is the maxLength of both in openapi.yaml, so a
// request the API schema accepts is never refused here.
const maxTransferNameLength = 1024

// transferJobNaming is what CreateJob derives from a request before it
// touches the queue: the cleaned target path and the names that fall back
// to it when the request left them blank.
type transferJobNaming struct {
	cleaned   string
	name      string
	batchName string
	batchKind TransferKind
}

// normalizeCreateTransferJob checks a create request and derives the names:
// the job's shape as a restored queue checks it, then what only a request
// carries. It is side-effect free so that the checks run before jobsMutex is
// taken.
func normalizeCreateTransferJob(input CreateTransferJob) (transferJobNaming, error) {
	if input.LastModified < 0 {
		return transferJobNaming{}, ErrInvalidTransfer
	}
	cleaned, err := validateTransferJobShape(input.shape())
	if err != nil {
		return transferJobNaming{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = path.Base(cleaned)
	}
	if len(name) > maxTransferNameLength {
		return transferJobNaming{}, ErrInvalidTransfer
	}
	batchName := strings.TrimSpace(input.BatchName)
	if batchName == "" {
		batchName = name
	}
	batchKind := input.BatchKind
	if batchKind == "" {
		batchKind = input.Kind
	}
	if len(batchName) > maxTransferNameLength || (batchKind != TransferFile && batchKind != TransferFolder) {
		return transferJobNaming{}, ErrInvalidTransfer
	}
	return transferJobNaming{cleaned: cleaned, name: name, batchName: batchName, batchKind: batchKind}, nil
}

// evictForAdmissionLocked makes room for one more job once the retained
// queue is full. A finished job that left nothing on the remote is removed
// in place and its id returned, so that the caller releases a download spool
// the job may still hold once the queue without it is saved. Otherwise the
// returned upload must have its remote part file cleaned up before it can go.
// An empty id with a nil cleanup means nothing can go.
func (m *TransferManager) evictForAdmissionLocked() (evictedID string, cleanup *transferJobRecord) {
	// Prefer records which have no remote partial state. A temporarily
	// unreachable upload tombstone must not freeze admission while a completed
	// transfer can be discarded without network I/O.
	for index, id := range m.jobOrder {
		record := m.jobs[id]
		if record == nil || record.cleanupInFlight || !evictableTransferJob(record.job) || mayHoldRemotePart(record.job) {
			continue
		}
		delete(m.jobs, id)
		m.jobOrder = append(m.jobOrder[:index], m.jobOrder[index+1:]...)
		return id, nil
	}
	for _, id := range m.jobOrder {
		record := m.jobs[id]
		if record != nil && !record.cleanupInFlight && mayHoldRemotePart(record.job) && record.job.Problem == CleanupPendingProblem {
			return "", record
		}
	}
	for index, id := range m.jobOrder {
		record := m.jobs[id]
		if record == nil || record.cleanupInFlight || !evictableTransferJob(record.job) {
			continue
		}
		if mayHoldRemotePart(record.job) {
			return "", record
		}
		delete(m.jobs, id)
		m.jobOrder = append(m.jobOrder[:index], m.jobOrder[index+1:]...)
		return id, nil
	}
	return "", nil
}

// CreateJob admits a job to the queue. The engine runs a remote job itself, so
// CreateJob starts it here, as UpdateJobFromClient does on resume and retry
// and the restored queue does on startup. A repeated request for a job
// already admitted starts nothing new: a live run is kept, and a job that is
// no longer queued refuses to start.
func (m *TransferManager) CreateJob(input CreateTransferJob) (TransferJob, error) {
	if input.Operation == RemoteGet || input.Operation == RemotePut {
		m.localMutationsMutex.Lock()
		defer m.localMutationsMutex.Unlock()
	}
	job, err := m.admitJob(input)
	if err == nil && job.Direction == TransferRemote {
		m.scheduleRemoteJob(job.ID)
	}
	return job, err
}

// admitJob records the job as queued, evicting a retained one when the queue
// is full.
func (m *TransferManager) admitJob(input CreateTransferJob) (TransferJob, error) {
	if m.isClosed() {
		return TransferJob{}, ErrUnavailable
	}
	m.sweepPreparedDownloads()
	naming, err := normalizeCreateTransferJob(input)
	if err != nil {
		return TransferJob{}, err
	}
	cleaned, name, batchName, batchKind := naming.cleaned, naming.name, naming.batchName, naming.batchKind

	m.jobsMutex.Lock()
	m.initializeJobsLocked()
	staleRemotes, err := m.sweepStaleJobsLocked()
	if err != nil {
		m.jobsMutex.Unlock()
		return TransferJob{}, err
	}
	if existing := m.jobs[input.ID]; existing != nil {
		if sameTransferIdentity(existing.job, input, cleaned, name) {
			job := cloneTransferJob(existing.job)
			m.jobsMutex.Unlock()
			closeRemotes(staleRemotes)
			return job, nil
		}
		m.jobsMutex.Unlock()
		closeRemotes(staleRemotes)
		return TransferJob{}, ErrConflict
	}
	admissionSnapshot := m.snapshotTransferQueueLocked()
	evictedID := ""
	if len(m.jobOrder) >= maxRetainedTransferJobs {
		var cleanup *transferJobRecord
		if evictedID, cleanup = m.evictForAdmissionLocked(); cleanup != nil {
			originalCleanup := cloneTransferJobRecord(cleanup)
			cleanup.cleanupInFlight = true
			cleanup.cleanupTombstone = true
			cleanup.job.Problem = CleanupPendingProblem
			cleanup.job.UpdatedAt = m.now().UTC()
			if persistErr := m.persistJobsLocked(true); persistErr != nil {
				*cleanup = originalCleanup
				m.jobsMutex.Unlock()
				closeRemotes(staleRemotes)
				return TransferJob{}, persistErr
			}
			cleanupJob := cleanup.job
			m.jobsMutex.Unlock()
			closeRemotes(staleRemotes)
			cleanupErr := m.cleanupTransferPart(cleanupJob)
			m.jobsMutex.Lock()
			var persistErr error
			if current := m.jobs[cleanupJob.ID]; current == cleanup {
				current.cleanupInFlight = false
				if cleanupErr == nil {
					orderIndex := m.jobOrderIndexLocked(cleanupJob.ID)
					delete(m.jobs, cleanupJob.ID)
					m.removeJobOrderLocked(cleanupJob.ID)
					if persistErr = m.persistJobsLocked(true); persistErr != nil {
						m.jobs[cleanupJob.ID] = current
						m.insertJobOrderLocked(orderIndex, cleanupJob.ID)
					}
				}
			}
			m.jobsMutex.Unlock()
			if cleanupErr != nil {
				return TransferJob{}, ErrTransferLimit
			}
			if persistErr != nil {
				return TransferJob{}, persistErr
			}
			return m.admitJob(input)
		}
		if len(m.jobOrder) >= maxRetainedTransferJobs {
			m.jobsMutex.Unlock()
			closeRemotes(staleRemotes)
			return TransferJob{}, ErrTransferLimit
		}
	}
	now := m.now().UTC()
	job := TransferJob{
		ID: input.ID, BatchID: input.BatchID, BatchName: batchName, BatchKind: batchKind,
		Alias: input.Alias, SourceAlias: input.SourceAlias, SourcePath: input.SourcePath,
		Operation: input.Operation, Direction: input.Direction,
		Kind: input.Kind, Name: name, RemotePath: cleaned, TotalBytes: input.TotalBytes,
		LastModified: input.LastModified, Overwrite: input.Overwrite,
		LargeFileThresholdBytes: input.LargeFileThresholdBytes, LargeFileParallelism: input.LargeFileParallelism,
		LargeFileChunkBytes: input.LargeFileChunkBytes,
		Status:              TransferQueued, Attempt: 1, CreatedAt: now, UpdatedAt: now,
		RemainingSeconds: -1,
	}
	if input.Kind == TransferFolder && input.Operation != RemoteMove && input.Operation != RemoteDelete {
		job.ExcludePatterns = append([]string(nil), m.excludePatterns...)
	}
	m.jobs[input.ID] = &transferJobRecord{job: job, sampleAt: now}
	m.jobOrder = append(m.jobOrder, input.ID)
	if persistErr := m.persistJobsLocked(true); persistErr != nil {
		// The restored queue has the evicted job back, spool included.
		m.restoreTransferQueueLocked(admissionSnapshot)
		m.jobsMutex.Unlock()
		closeRemotes(staleRemotes)
		return TransferJob{}, persistErr
	}
	m.jobsMutex.Unlock()
	closeRemotes(staleRemotes)
	if evictedID != "" {
		m.releasePreparedDownload(evictedID)
	}
	return cloneTransferJob(job), nil
}

// sameTransferIdentity reports whether a resent CreateJob asks for the job
// already admitted under its ID. The large-file split is left out: it says how
// the job transfers, not which transfer it is, and StartOwned replaces the
// values an upload left at zero with the engine's, so a resend of the original
// request would no longer match them.
func sameTransferIdentity(job TransferJob, input CreateTransferJob, cleaned, name string) bool {
	batchName := strings.TrimSpace(input.BatchName)
	if batchName == "" {
		batchName = name
	}
	batchKind := input.BatchKind
	if batchKind == "" {
		batchKind = input.Kind
	}
	return job.BatchID == input.BatchID && job.Alias == input.Alias && job.Direction == input.Direction &&
		job.SourceAlias == input.SourceAlias && job.SourcePath == input.SourcePath && job.Operation == input.Operation &&
		job.Kind == input.Kind && job.Name == name && job.RemotePath == cleaned && job.TotalBytes == input.TotalBytes &&
		job.BatchName == batchName && job.BatchKind == batchKind && job.LastModified == input.LastModified && job.Overwrite == input.Overwrite
}

func evictableTransferJob(job TransferJob) bool {
	return retainedTransferStatus(job.Status) && !(mayHoldRemotePart(job) && job.Problem == CleanupPendingProblem)
}
