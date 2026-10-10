package sftp

import (
	"strings"
	"time"
)

// transferOutcomeUnrecorded reports a stopped job that still carries
// RemoteReconciliationProblem: its operation may have changed the destination
// without the result reaching the queue. The allowed actions and the update
// validation both use this one predicate, so the UI never offers a resume that
// the engine then refuses. A running job carries the same problem while its
// operation is in flight; its worker clears it when a pause interrupts the
// operation before the result.
func transferOutcomeUnrecorded(job TransferJob) bool {
	return job.Problem == RemoteReconciliationProblem && job.Status != TransferRunning && job.Status != TransferReconnecting
}

type TransferJobAction string

const (
	TransferStartAction          TransferJobAction = "start"
	TransferReconnectAction      TransferJobAction = "reconnect"
	TransferPauseAction          TransferJobAction = "pause"
	TransferResumeAction         TransferJobAction = "resume"
	TransferRetryAction          TransferJobAction = "retry"
	TransferCancelAction         TransferJobAction = "cancel"
	TransferProgressAction       TransferJobAction = "progress"
	TransferCompleteAction       TransferJobAction = "complete"
	TransferFailAction           TransferJobAction = "fail"
	TransferNeedsOverwriteAction TransferJobAction = "needs_overwrite"
)

type TransferControlAction string

const (
	TransferPauseControl  TransferControlAction = "pause"
	TransferResumeControl TransferControlAction = "resume"
	TransferRetryControl  TransferControlAction = "retry"
	TransferCancelControl TransferControlAction = "cancel"
	TransferRemoveControl TransferControlAction = "remove"
)

// AllowedTransferActions returns the user-visible control actions permitted by
// the engine state machine. Data-plane-only actions such as progress and
// complete intentionally remain private to the transfer implementation.
func AllowedTransferActions(job TransferJob) []TransferControlAction {
	if transferOutcomeUnrecorded(job) {
		return []TransferControlAction{TransferCancelControl}
	}
	switch job.Status {
	case TransferQueued, TransferRunning, TransferReconnecting:
		return []TransferControlAction{TransferPauseControl, TransferCancelControl}
	case TransferPaused, TransferReattach, TransferNeedsOverwrite:
		return []TransferControlAction{TransferResumeControl, TransferCancelControl}
	case TransferFailed:
		return []TransferControlAction{TransferRetryControl, TransferCancelControl, TransferRemoveControl}
	case TransferCompleted, TransferCancelled:
		return []TransferControlAction{TransferRemoveControl}
	default:
		return []TransferControlAction{}
	}
}

type transferUpdateOrigin uint8

const (
	transferUpdateInternal transferUpdateOrigin = iota
	transferUpdateClient
	transferUpdateUploadData
)

// transferUpdateRequest is one change updateJob applies to the ledger.
type transferUpdateRequest struct {
	id     string
	update UpdateTransferJob
	origin transferUpdateOrigin
	// run is set only when a remote worker reports on its own run. A run
	// cancelled by pause, cancel or engine shutdown no longer owns the job, so
	// its late reports are refused instead of overwriting a resumed job.
	run *remoteRun
}

func (m *TransferManager) UpdateJob(id string, update UpdateTransferJob) (TransferJob, error) {
	if m.isClosed() {
		return TransferJob{}, ErrUnavailable
	}
	return m.updateJob(transferUpdateRequest{id: id, update: update, origin: transferUpdateInternal})
}

// UpdateJobFromClient accepts queue controls but reserves upload progress and
// terminal publication state for the owned data-plane endpoints.
func (m *TransferManager) UpdateJobFromClient(id string, update UpdateTransferJob) (TransferJob, error) {
	if m.isClosed() {
		return TransferJob{}, ErrUnavailable
	}
	// The same operation lock is held by upload publication. A pause/fail cannot
	// change the queue state between the atomic rename and its terminal commit.
	unlock := m.lock("", jobOwnerLockKey(id))
	defer unlock()
	job, err := m.updateJob(transferUpdateRequest{id: id, update: update, origin: transferUpdateClient})
	if err == nil && job.Direction == TransferDownload && (update.Action == TransferCancelAction || update.Action == TransferCompleteAction) {
		m.releasePreparedDownload(id)
	}
	// updateJob has already ended the run of a remote job that was paused or
	// cancelled.
	if err == nil && job.Direction == TransferRemote &&
		(update.Action == TransferResumeAction || update.Action == TransferRetryAction) {
		m.scheduleRemoteJob(id)
	}
	return job, err
}

// updateUploadJob is reserved for the upload data plane. Keeping this entry
// point private makes the server, rather than a browser-supplied follow-up
// request, authoritative for upload progress and terminal state.
func (m *TransferManager) updateUploadJob(id string, update UpdateTransferJob) (TransferJob, error) {
	return m.updateJob(transferUpdateRequest{id: id, update: update, origin: transferUpdateUploadData})
}

func (m *TransferManager) updateJob(request transferUpdateRequest) (TransferJob, error) {
	id, update, origin := request.id, request.update, request.origin
	if !transferIDPattern.MatchString(id) {
		return TransferJob{}, ErrInvalidTransfer
	}
	m.jobsMutex.Lock()
	m.initializeJobsLocked()
	staleRemotes, err := m.sweepStaleJobsLocked()
	if err != nil {
		m.jobsMutex.Unlock()
		return TransferJob{}, err
	}
	var terminalRemote Remote
	defer func() {
		m.jobsMutex.Unlock()
		closeRemotes(staleRemotes)
		if terminalRemote != nil {
			_ = terminalRemote.Close()
		}
	}()
	record := m.jobs[id]
	if record == nil {
		return TransferJob{}, ErrTransferNotFound
	}
	if err := request.run.ownershipError(); err != nil {
		return TransferJob{}, err
	}
	if record.cleanupInFlight || (record.cleanupTombstone && !(origin == transferUpdateUploadData && update.Action == TransferCancelAction)) {
		return TransferJob{}, ErrTransferState
	}
	original := cloneTransferJobRecord(record)
	originalActive := m.activeJobs
	committed := false
	defer func() {
		if !committed {
			*record = original
			m.activeJobs = originalActive
		}
	}()
	job := &record.job
	if origin == transferUpdateUploadData && job.Direction != TransferUpload {
		return TransferJob{}, ErrConflict
	}
	if err := validateTransferUpdate(*job, update, origin); err != nil {
		return TransferJob{}, err
	}
	now := m.now().UTC()

	settled, err := m.applyTransferActionLocked(record, update, now)
	if err != nil {
		return TransferJob{}, err
	}
	if settled {
		committed = true
		return cloneTransferJob(*job), nil
	}
	if job.Status != TransferReconnecting {
		job.ReconnectAt = time.Time{}
	}
	job.UpdatedAt = now
	if err := m.persistJobsLocked(update.Action != TransferProgressAction); err != nil {
		return TransferJob{}, err
	}
	if job.Status != TransferRunning && job.Status != TransferQueued && job.Status != TransferReconnecting {
		terminalRemote = m.detachRemote(job.Alias, job.ID, job.RemotePath)
		if job.Direction == TransferRemote {
			// The run has finished with the job, whether its worker settled it
			// or a client paused or cancelled it. Ending the run while jobsMutex
			// still holds the new state refuses every later report of that
			// worker, so an operation that returns after a pause is treated as
			// interrupted, and a retry or resume that follows at once starts a
			// fresh worker instead of finding this one still registered.
			m.endRemoteRun(id)
		}
	}
	committed = true
	return cloneTransferJob(*job), nil
}

// applyTransferActionLocked moves a job through one queue action. It edits
// the record in place; updateJob restores the clone it took when anything
// after this fails. settled reports an action that found its work already
// done (cancel or complete on a finished job), which commits as is.
func (m *TransferManager) applyTransferActionLocked(record *transferJobRecord, update UpdateTransferJob, now time.Time) (settled bool, err error) {
	job := &record.job
	switch update.Action {
	case TransferStartAction:
		if job.Status == TransferReconnecting {
			if m.processingStopped || !m.autoReconnect || job.ReconnectAttempt > m.maxReconnectAttempts || now.Before(job.ReconnectAt) {
				return false, ErrTransferState
			}
			job.Status, job.Problem, job.ReconnectAt = TransferRunning, "", time.Time{}
			return false, nil
		}
		if job.Status != TransferQueued {
			return false, ErrTransferState
		}
		// 停止中は待機のまま置く。実行中のものは止めない。止めたければ
		// pause がある。
		if m.processingStopped {
			return false, ErrTransferLimit
		}
		if m.activeJobs >= m.maxConcurrent {
			return false, ErrTransferLimit
		}
		job.Status = TransferRunning
		m.activeJobs++
		record.sampleAt, record.sampleBytes = now, job.TransferredBytes
	case TransferReconnectAction:
		if job.Status != TransferRunning || !m.autoReconnect || m.processingStopped || job.ReconnectAttempt >= m.maxReconnectAttempts {
			return false, ErrTransferState
		}
		if job.Direction == TransferRemote && !recoverableRemoteFile(*job) {
			return false, ErrTransferState
		}
		job.ReconnectAttempt++
		job.Attempt++
		job.Status, job.Problem = TransferReconnecting, "sftp_connection_lost"
		job.ReconnectAt = now.Add(reconnectDelay(job.ReconnectAttempt))
		job.BytesPerSecond, job.RemainingSeconds = 0, -1
	case TransferPauseAction:
		if job.Status != TransferQueued && job.Status != TransferRunning && job.Status != TransferReconnecting {
			return false, ErrTransferState
		}
		m.releaseJobLocked(job.Status)
		job.Status = TransferPaused
	case TransferResumeAction:
		if job.Status != TransferPaused && job.Status != TransferReattach && job.Status != TransferNeedsOverwrite {
			return false, ErrTransferState
		}
		if update.ResetProgress {
			job.TransferredBytes = 0
			if job.Direction == TransferUpload {
				job.UploadRanges = nil
			}
			if job.Direction == TransferDownload {
				record.sentBytes, record.revision = 0, ""
			}
			job.DownloadRevision = ""
			job.BytesPerSecond, job.RemainingSeconds = 0, -1
			record.sampleAt, record.sampleBytes = now, 0
		}
		if job.Status == TransferNeedsOverwrite && (job.Direction == TransferUpload || job.Direction == TransferRemote) {
			job.Overwrite = true
		}
		job.Status = TransferQueued
		job.Problem = ""
		job.ReconnectAttempt, job.ReconnectAt = 0, time.Time{}
	case TransferRetryAction:
		if job.Status != TransferFailed {
			return false, ErrTransferState
		}
		if update.ResetProgress {
			job.TransferredBytes = 0
			if job.Direction == TransferUpload {
				job.UploadRanges = nil
			}
			if job.Direction == TransferDownload {
				record.sentBytes, record.revision = 0, ""
				job.DownloadRevision = ""
			}
		}
		job.Status, job.Problem = TransferQueued, ""
		job.ReconnectAttempt, job.ReconnectAt = 0, time.Time{}
		job.Attempt++
		job.BytesPerSecond, job.RemainingSeconds = 0, -1
		record.sampleAt, record.sampleBytes = now, job.TransferredBytes
	case TransferCancelAction:
		if terminalTransferStatus(job.Status) {
			// Cancel is idempotent across a completion race. A completed transfer
			// stays completed; callers only asked that no work remain active.
			return true, nil
		}
		m.releaseJobLocked(job.Status)
		job.Status, job.Problem = TransferCancelled, ""
		record.cleanupTombstone = false
	case TransferProgressAction:
		if update.ResetProgress {
			job.TransferredBytes = *update.TransferredBytes
			if job.Direction == TransferDownload {
				record.sentBytes, record.revision = 0, ""
			}
			if update.TotalBytes != nil {
				job.TotalBytes = *update.TotalBytes
			}
			job.BytesPerSecond, job.RemainingSeconds = 0, -1
			record.sampleAt, record.sampleBytes = now, job.TransferredBytes
		} else {
			job.TransferredBytes = *update.TransferredBytes
		}
		m.updateRateLocked(record, now)
	case TransferCompleteAction:
		if job.Status == TransferCompleted {
			return true, nil
		}
		if update.TransferredBytes != nil {
			job.TransferredBytes = *update.TransferredBytes
		}
		m.releaseJobLocked(job.Status)
		job.Status, job.Problem = TransferCompleted, ""
		job.RemainingSeconds = 0
	case TransferFailAction:
		if job.Status != TransferRunning && job.Status != TransferQueued && job.Status != TransferReconnecting {
			return false, ErrTransferState
		}
		m.releaseJobLocked(job.Status)
		job.Status, job.Problem = TransferFailed, boundedTransferProblem(update.Problem)
	case TransferNeedsOverwriteAction:
		if job.Status != TransferRunning && job.Status != TransferQueued && job.Status != TransferReconnecting {
			return false, ErrTransferState
		}
		m.releaseJobLocked(job.Status)
		job.Status, job.Problem = TransferNeedsOverwrite, "sftp_exists"
	default:
		return false, ErrInvalidTransfer
	}
	return false, nil
}

// validateTransferUpdate is deliberately side-effect free. Browser fields are
// checked together with direction and state while jobsMutex is held, before a
// single byte or slot counter can be mutated.
func validateTransferUpdate(job TransferJob, update UpdateTransferJob, origin transferUpdateOrigin) error {
	hasTransferred := update.TransferredBytes != nil
	hasTotal := update.TotalBytes != nil
	hasProblem := strings.TrimSpace(update.Problem) != ""
	noPayload := !hasTransferred && !hasTotal && !hasProblem && !update.ResetProgress
	if transferOutcomeUnrecorded(job) && (update.Action == TransferResumeAction || update.Action == TransferRetryAction) {
		return ErrTransferState
	}

	if origin == transferUpdateClient {
		if job.Direction == TransferRemote && update.Action == TransferReconnectAction {
			return ErrTransferState
		}
		if job.Direction == TransferUpload && (update.Action == TransferProgressAction || update.Action == TransferCompleteAction || update.Action == TransferCancelAction) {
			return ErrTransferState
		}
		if job.Direction == TransferDownload && update.Action == TransferProgressAction &&
			(!update.ResetProgress || !hasTransferred || *update.TransferredBytes != 0 || hasTotal || hasProblem) {
			return ErrTransferState
		}
		if job.Direction == TransferDownload && update.Action == TransferCompleteAction {
			if job.Status != TransferRunning || job.TotalBytes < 0 || job.TransferredBytes != job.TotalBytes ||
				(hasTransferred && *update.TransferredBytes != job.TransferredBytes) {
				return ErrTransferState
			}
		}
	}

	switch update.Action {
	case TransferStartAction, TransferReconnectAction, TransferPauseAction, TransferCancelAction, TransferNeedsOverwriteAction:
		if !noPayload {
			return ErrInvalidTransfer
		}
	case TransferResumeAction, TransferRetryAction:
		if hasTransferred || hasTotal || hasProblem {
			return ErrInvalidTransfer
		}
	case TransferProgressAction:
		if job.Status != TransferRunning || !hasTransferred || hasProblem {
			return ErrTransferState
		}
		if hasTotal && !update.ResetProgress {
			return ErrInvalidTransfer
		}
		if update.ResetProgress && *update.TransferredBytes != 0 && origin != transferUpdateUploadData {
			return ErrInvalidTransfer
		}
		if hasTotal && *update.TotalBytes < 0 {
			return ErrInvalidTransfer
		}
		transferred := *update.TransferredBytes
		if update.ResetProgress && (transferred < 0 || (update.TotalBytes != nil && transferred > *update.TotalBytes)) {
			return ErrOffsetMismatch
		}
		if !update.ResetProgress && (transferred < job.TransferredBytes || transferred < 0 || (job.TotalBytes >= 0 && transferred > job.TotalBytes)) {
			return ErrOffsetMismatch
		}
	case TransferCompleteAction:
		if hasTotal || hasProblem || update.ResetProgress {
			return ErrInvalidTransfer
		}
		transferred := job.TransferredBytes
		if hasTransferred {
			transferred = *update.TransferredBytes
			if transferred < job.TransferredBytes || transferred < 0 || (job.TotalBytes >= 0 && transferred > job.TotalBytes) {
				return ErrOffsetMismatch
			}
		}
		if job.Status != TransferCompleted && (job.Status != TransferRunning || (job.TotalBytes >= 0 && transferred != job.TotalBytes)) {
			return ErrTransferState
		}
	case TransferFailAction:
		if hasTransferred || hasTotal || update.ResetProgress {
			return ErrInvalidTransfer
		}
	default:
		return ErrInvalidTransfer
	}
	return nil
}

// transferRateSmoothing is the weight of the newest progress sample in the
// speed a job shows; the rest stays with the speed shown before. The speed and
// the time left then do not jump with every report, yet follow a real change
// within a few reports.
const transferRateSmoothing = 0.35

// maxTransferProblemLength is the maxLength of problem in openapi.yaml. A
// problem is a short code such as sftp_failed; a longer one from a client is
// cut so that a listed job stays within the schema.
const maxTransferProblemLength = 128

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
		record.job.BytesPerSecond = record.job.BytesPerSecond*(1-transferRateSmoothing) + instant*transferRateSmoothing
	}
	if record.job.TotalBytes >= 0 && record.job.BytesPerSecond > 0 {
		remaining := float64(record.job.TotalBytes-record.job.TransferredBytes) / record.job.BytesPerSecond
		record.job.RemainingSeconds = int64(remaining + 0.5)
	} else {
		record.job.RemainingSeconds = -1
	}
	record.sampleAt, record.sampleBytes = now, record.job.TransferredBytes
}

func (m *TransferManager) releaseJobLocked(status TransferJobStatus) {
	if (status == TransferRunning || status == TransferReconnecting) && m.activeJobs > 0 {
		m.activeJobs--
		m.signalSlotLocked()
	}
}

func boundedTransferProblem(problem string) string {
	problem = strings.TrimSpace(problem)
	if problem == "" {
		return "sftp_failed"
	}
	if len(problem) > maxTransferProblemLength {
		return problem[:maxTransferProblemLength]
	}
	return problem
}
