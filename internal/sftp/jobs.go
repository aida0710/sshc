package sftp

import (
	"context"
	"path"
	"strings"
	"time"
)

const (
	DefaultTransferConcurrency  = 2
	MaxTransferConcurrency      = 8
	DefaultLargeFileThreshold   = int64(100 << 20)
	MinLargeFileThreshold       = int64(16 << 20)
	MaxLargeFileThreshold       = int64(1 << 30)
	DefaultLargeFileParallelism = 4
	MaxLargeFileParallelism     = 8
	DefaultLargeFileChunkBytes  = int64(32 << 20)
	MinLargeFileChunkBytes      = int64(8 << 20)
	MaxLargeFileChunkBytes      = int64(4 << 30)
	maxRetainedTransferJobs     = 200
	staleRunningTransferAfter   = 2 * time.Minute

	// 完了項目の自動消去は、押し忘れても消える程度に長く、履歴として頼れる
	// ほどには短い範囲に収める。0 は自動消去なしである。
	MinClearCompletedAfter = 10 * time.Second
	MaxClearCompletedAfter = 24 * time.Hour
)

// TransferQueueMove は、待機中の job を待機列の中でどこへ動かすかである。
// running の job は動かさない。動かせば、いま走っているものが止まる。
type TransferQueueMove string

const (
	TransferMoveUp     TransferQueueMove = "up"
	TransferMoveDown   TransferQueueMove = "down"
	TransferMoveTop    TransferQueueMove = "top"
	TransferMoveBottom TransferQueueMove = "bottom"
)

type TransferDirection string

const (
	TransferUpload   TransferDirection = "upload"
	TransferDownload TransferDirection = "download"
	TransferRemote   TransferDirection = "remote"
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

type TransferControlAction string

const (
	TransferPauseControl  TransferControlAction = "pause"
	TransferResumeControl TransferControlAction = "resume"
	TransferRetryControl  TransferControlAction = "retry"
	TransferCancelControl TransferControlAction = "cancel"
)

// AllowedTransferActions returns the user-visible control actions permitted by
// the engine state machine. Data-plane-only actions such as progress and
// complete intentionally remain private to the transfer implementation.
func AllowedTransferActions(job TransferJob) []TransferControlAction {
	switch job.Status {
	case TransferQueued, TransferRunning:
		return []TransferControlAction{TransferPauseControl, TransferCancelControl}
	case TransferPaused, TransferReattach, TransferNeedsOverwrite:
		return []TransferControlAction{TransferResumeControl, TransferCancelControl}
	case TransferFailed:
		return []TransferControlAction{TransferRetryControl, TransferCancelControl}
	default:
		return []TransferControlAction{}
	}
}

type TransferJob struct {
	ID                      string
	BatchID                 string
	BatchName               string
	BatchKind               TransferKind
	Alias                   string
	SourceAlias             string
	SourcePath              string
	Operation               RemoteTransferOperation
	Direction               TransferDirection
	Kind                    TransferKind
	Name                    string
	RemotePath              string
	TotalBytes              int64
	TransferredBytes        int64
	BytesPerSecond          float64
	RemainingSeconds        int64
	Status                  TransferJobStatus
	Attempt                 int
	Problem                 string
	LastModified            int64
	ExpectedRevision        string
	SourceFingerprint       string
	Overwrite               bool
	DownloadRevision        string
	LargeFileThresholdBytes int64
	LargeFileParallelism    int
	LargeFileChunkBytes     int64
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

type CreateTransferJob struct {
	ID                      string
	BatchID                 string
	BatchName               string
	BatchKind               TransferKind
	Alias                   string
	SourceAlias             string
	SourcePath              string
	Operation               RemoteTransferOperation
	Overwrite               bool
	Direction               TransferDirection
	Kind                    TransferKind
	Name                    string
	RemotePath              string
	TotalBytes              int64
	LastModified            int64
	LargeFileThresholdBytes int64
	LargeFileParallelism    int
	LargeFileChunkBytes     int64
}

type UpdateTransferJob struct {
	Action           TransferJobAction
	TransferredBytes *int64
	TotalBytes       *int64
	Problem          string
	ResetProgress    bool
}

type transferJobRecord struct {
	job              TransferJob
	sampleAt         time.Time
	sampleBytes      int64
	sentBytes        int64
	revision         string
	cleanupInFlight  bool
	cleanupTombstone bool
}

type transferUpdateOrigin uint8

const (
	transferUpdateInternal transferUpdateOrigin = iota
	transferUpdateClient
	transferUpdateUploadData
)

// AuthorizeUpload binds the data-plane upload endpoints to the queue record
// that owns their slot. Without this check, callers could append or complete a
// part file without ever acquiring one of the backend concurrency slots.
func (m *TransferManager) AuthorizeUpload(id, alias, remotePath string, total int64, cancelling bool) error {
	cleaned, err := cleanPublicPath(remotePath, false)
	if err != nil || !transferIDPattern.MatchString(id) {
		return ErrInvalidTransfer
	}
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	record := m.jobs[id]
	if record == nil {
		return ErrTransferNotFound
	}
	job := record.job
	if job.Direction != TransferUpload || job.Kind != TransferFile || job.Alias != alias ||
		job.RemotePath != cleaned || (total >= 0 && job.TotalBytes != total) {
		return ErrConflict
	}
	if cancelling {
		if job.Status == TransferCompleted {
			return ErrTransferState
		}
		return nil
	}
	if job.Status != TransferRunning {
		return ErrTransferState
	}
	return nil
}

// AuthorizeDownload binds a GET data-plane request to a running download job
// which already owns a shared queue slot.
func (m *TransferManager) AuthorizeDownload(id, alias, remotePath string, kind TransferKind) (TransferJob, error) {
	cleaned, err := cleanPublicPath(remotePath, false)
	if err != nil || !transferIDPattern.MatchString(id) {
		return TransferJob{}, ErrInvalidTransfer
	}
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	record := m.jobs[id]
	if record == nil {
		return TransferJob{}, ErrTransferNotFound
	}
	job := record.job
	if job.Direction != TransferDownload || job.Kind != kind || job.Alias != alias || job.RemotePath != cleaned {
		return TransferJob{}, ErrConflict
	}
	if job.Status != TransferRunning {
		return TransferJob{}, ErrTransferState
	}
	return job, nil
}

// StartDownloadDataPlane validates ownership and marks the operation active in
// one jobsMutex critical section. Cancel cannot slip between authorization and
// the stale-sweep protection token.
func (m *TransferManager) StartDownloadDataPlane(id, alias, remotePath string, kind TransferKind) (TransferJob, func(), error) {
	cleaned, err := cleanPublicPath(remotePath, false)
	if err != nil || !transferIDPattern.MatchString(id) {
		return TransferJob{}, nil, ErrInvalidTransfer
	}
	m.jobsMutex.Lock()
	m.initializeJobsLocked()
	record := m.jobs[id]
	if record == nil {
		m.jobsMutex.Unlock()
		return TransferJob{}, nil, ErrTransferNotFound
	}
	job := record.job
	if job.Direction != TransferDownload || job.Kind != kind || job.Alias != alias || job.RemotePath != cleaned {
		m.jobsMutex.Unlock()
		return TransferJob{}, nil, ErrConflict
	}
	if job.Status != TransferRunning {
		m.jobsMutex.Unlock()
		return TransferJob{}, nil, ErrTransferState
	}
	m.dataPlane[id]++
	m.jobsMutex.Unlock()
	done := func() {
		m.jobsMutex.Lock()
		if m.dataPlane[id] <= 1 {
			delete(m.dataPlane, id)
		} else {
			m.dataPlane[id]--
		}
		m.jobsMutex.Unlock()
	}
	return job, done, nil
}

// BeginDownload pins one response to an opaque content revision. A resumed
// response may start only at the browser's last durable checkpoint; bytes that
// were merely written to a socket are tracked separately in sentBytes.
func (m *TransferManager) BeginDownload(id string, total int64, revision string, offset int64) (TransferJob, error) {
	if !transferIDPattern.MatchString(id) || revision == "" || total < -1 || offset < 0 {
		return TransferJob{}, ErrInvalidTransfer
	}
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	record := m.jobs[id]
	if record == nil {
		return TransferJob{}, ErrTransferNotFound
	}
	job := &record.job
	if job.Direction != TransferDownload || job.Status != TransferRunning {
		return TransferJob{}, ErrTransferState
	}
	changed := record.revision != revision || (total >= 0 && job.TotalBytes != total)
	if changed {
		record.revision, record.sentBytes = revision, 0
		job.DownloadRevision = revision
		job.TransferredBytes = 0
		job.TotalBytes = total
		job.BytesPerSecond, job.RemainingSeconds = 0, -1
		record.sampleAt, record.sampleBytes = m.now().UTC(), 0
	}
	job.DownloadRevision = record.revision
	if offset != job.TransferredBytes || offset > record.sentBytes {
		return TransferJob{}, ErrOffsetMismatch
	}
	job.UpdatedAt = m.now().UTC()
	_ = m.persistJobsLocked(true)
	return *job, nil
}

// RecordDownloadSent records a server-observed socket high-water mark. It does
// not advance TransferredBytes: only a durable browser checkpoint ACK may do
// that, so a crash can safely request retransmission of an already-sent suffix.
func (m *TransferManager) RecordDownloadSent(id string, sent, total int64, revision string) (TransferJob, error) {
	if !transferIDPattern.MatchString(id) || revision == "" || sent < 0 || total < -1 {
		return TransferJob{}, ErrInvalidTransfer
	}
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	record := m.jobs[id]
	if record == nil {
		return TransferJob{}, ErrTransferNotFound
	}
	job := &record.job
	if job.Direction != TransferDownload || job.Status != TransferRunning || record.revision != revision {
		return TransferJob{}, ErrTransferState
	}
	if total >= 0 {
		if sent > total {
			return TransferJob{}, ErrOffsetMismatch
		}
		job.TotalBytes = total
	}
	if sent > record.sentBytes {
		record.sentBytes = sent
	}
	job.UpdatedAt = m.now().UTC()
	_ = m.persistJobsLocked(false)
	return *job, nil
}

// VerifyDownloadComplete confirms that an earlier response from this engine
// actually sent the whole pinned revision. A metadata-only verify request must
// never manufacture sentBytes after an engine restart.
func (m *TransferManager) VerifyDownloadComplete(id string, total int64, revision string) (TransferJob, error) {
	if !transferIDPattern.MatchString(id) || revision == "" || total < 0 {
		return TransferJob{}, ErrInvalidTransfer
	}
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	record := m.jobs[id]
	if record == nil {
		return TransferJob{}, ErrTransferNotFound
	}
	job := &record.job
	if job.Direction != TransferDownload || job.Status != TransferRunning || record.revision != revision {
		return TransferJob{}, ErrTransferState
	}
	if job.TotalBytes != total || record.sentBytes != total {
		return TransferJob{}, ErrOffsetMismatch
	}
	job.UpdatedAt = m.now().UTC()
	_ = m.persistJobsLocked(true)
	return *job, nil
}

// AcknowledgeDownload advances (or reconciles backwards) to an OPFS-durable
// offset only when it belongs to the currently pinned response revision and is
// no larger than bytes the server actually sent.
func (m *TransferManager) AcknowledgeDownload(id string, offset int64, revision string) (TransferJob, error) {
	if !transferIDPattern.MatchString(id) || revision == "" || offset < 0 {
		return TransferJob{}, ErrInvalidTransfer
	}
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	record := m.jobs[id]
	if record == nil {
		return TransferJob{}, ErrTransferNotFound
	}
	job := &record.job
	if job.Direction != TransferDownload || job.Status != TransferRunning || record.revision != revision {
		return TransferJob{}, ErrTransferState
	}
	if offset > record.sentBytes || (job.TotalBytes >= 0 && offset > job.TotalBytes) {
		return TransferJob{}, ErrOffsetMismatch
	}
	now := m.now().UTC()
	if offset < job.TransferredBytes {
		job.BytesPerSecond, job.RemainingSeconds = 0, -1
		record.sampleAt, record.sampleBytes = now, offset
	}
	job.TransferredBytes = offset
	m.updateRateLocked(record, now)
	job.UpdatedAt = now
	_ = m.persistJobsLocked(false)
	return *job, nil
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
	m.largeFileThreshold = DefaultLargeFileThreshold
	m.largeFileParallelism = DefaultLargeFileParallelism
	m.largeFileChunkBytes = DefaultLargeFileChunkBytes
	m.now = now
	if m.jobs == nil {
		m.jobs = make(map[string]*transferJobRecord)
	}
	if m.dataPlane == nil {
		m.dataPlane = make(map[string]int)
	}
}

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

func (m *TransferManager) MaxConcurrent() int {
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	return m.maxConcurrent
}

// ClearCompletedAfter は、完了と取消の記録を自動で消すまでの時間である。
// 0 は自動消去なしを意味する。
func (m *TransferManager) ClearCompletedAfter() time.Duration {
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	return m.clearCompletedAfter
}

// ProcessingStopped は、待機中の job を新しく開始しない状態かどうかである。
func (m *TransferManager) ProcessingStopped() bool {
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	return m.processingStopped
}

func (m *TransferManager) LargeFileThreshold() int64 {
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	return m.largeFileThreshold
}

func (m *TransferManager) LargeFileParallelism() int {
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	return m.largeFileParallelism
}

func (m *TransferManager) LargeFileChunkBytes() int64 {
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	return m.largeFileChunkBytes
}

// SetTransferSettings は、同時転送数、完了項目の自動消去時間、キュー処理の
// 停止を差し替える。
//
// 転送は engine の資源であって browser のものではないため、値は engine 側に
// 一つだけ置く。永続化は呼び出し側の責務である。
func (m *TransferManager) SetTransferSettings(
	maxConcurrent int, clearCompletedAfter time.Duration, processingStopped bool,
	largeFileThreshold int64, largeFileParallelism int, largeFileChunkBytes int64,
) error {
	if maxConcurrent < 1 || maxConcurrent > MaxTransferConcurrency {
		return ErrInvalidTransfer
	}
	if clearCompletedAfter != 0 && (clearCompletedAfter < MinClearCompletedAfter || clearCompletedAfter > MaxClearCompletedAfter) {
		return ErrInvalidTransfer
	}
	if largeFileThreshold < MinLargeFileThreshold || largeFileThreshold > MaxLargeFileThreshold ||
		largeFileParallelism < 1 || largeFileParallelism > MaxLargeFileParallelism ||
		largeFileChunkBytes < MinLargeFileChunkBytes || largeFileChunkBytes > MaxLargeFileChunkBytes {
		return ErrInvalidTransfer
	}
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	m.maxConcurrent = maxConcurrent
	m.clearCompletedAfter = clearCompletedAfter
	m.processingStopped = processingStopped
	m.largeFileThreshold = largeFileThreshold
	m.largeFileParallelism = largeFileParallelism
	m.largeFileChunkBytes = largeFileChunkBytes
	return nil
}

// MoveQueuedJob は、待機中の job だけを待機列の中で入れ替える。running や
// paused の位置は動かないので、並べ替えても走っている転送は影響を受けない。
func (m *TransferManager) MoveQueuedJob(id string, move TransferQueueMove) error {
	if !transferIDPattern.MatchString(id) {
		return ErrInvalidTransfer
	}
	switch move {
	case TransferMoveUp, TransferMoveDown, TransferMoveTop, TransferMoveBottom:
	default:
		return ErrInvalidTransfer
	}
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	record := m.jobs[id]
	if record == nil {
		return ErrTransferNotFound
	}
	if record.job.Status != TransferQueued {
		return ErrTransferState
	}
	slots := make([]int, 0, len(m.jobOrder))
	current := -1
	for index, candidate := range m.jobOrder {
		waiting := m.jobs[candidate]
		if waiting == nil || waiting.job.Status != TransferQueued {
			continue
		}
		if candidate == id {
			current = len(slots)
		}
		slots = append(slots, index)
	}
	if current < 0 || len(slots) < 2 {
		return nil
	}
	destination := current
	switch move {
	case TransferMoveUp:
		destination = current - 1
	case TransferMoveDown:
		destination = current + 1
	case TransferMoveTop:
		destination = 0
	case TransferMoveBottom:
		destination = len(slots) - 1
	}
	if destination < 0 || destination >= len(slots) || destination == current {
		return nil
	}
	// 待機中の id だけを取り出し、順番を変えて同じ位置へ書き戻す。running の
	// job が占める index は触らないので、待機列だけが並び替わる。
	waiting := make([]string, 0, len(slots))
	for _, index := range slots {
		waiting = append(waiting, m.jobOrder[index])
	}
	moved := waiting[current]
	waiting = append(waiting[:current], waiting[current+1:]...)
	waiting = append(waiting[:destination], append([]string{moved}, waiting[destination:]...)...)
	for position, index := range slots {
		m.jobOrder[index] = waiting[position]
	}
	_ = m.persistJobsLocked(true)
	return nil
}

func (m *TransferManager) CreateJob(input CreateTransferJob) (TransferJob, error) {
	if m.isClosed() {
		return TransferJob{}, ErrUnavailable
	}
	m.sweepPreparedDownloads()
	if !transferIDPattern.MatchString(input.ID) || !transferIDPattern.MatchString(input.BatchID) ||
		strings.TrimSpace(input.Alias) == "" || len(input.Alias) > 255 || input.TotalBytes < -1 ||
		(input.Direction != TransferUpload && input.Direction != TransferDownload && input.Direction != TransferRemote) ||
		(input.Kind != TransferFile && input.Kind != TransferFolder) || input.LastModified < 0 {
		return TransferJob{}, ErrInvalidTransfer
	}
	if (input.LargeFileThresholdBytes != 0 &&
		(input.LargeFileThresholdBytes < MinLargeFileThreshold || input.LargeFileThresholdBytes > MaxLargeFileThreshold)) ||
		(input.LargeFileParallelism != 0 &&
			(input.LargeFileParallelism < 1 || input.LargeFileParallelism > MaxLargeFileParallelism)) ||
		(input.LargeFileChunkBytes != 0 &&
			(input.LargeFileChunkBytes < MinLargeFileChunkBytes || input.LargeFileChunkBytes > MaxLargeFileChunkBytes)) ||
		((input.Direction != TransferDownload || input.Kind != TransferFile) &&
			(input.LargeFileThresholdBytes != 0 || input.LargeFileParallelism != 0 || input.LargeFileChunkBytes != 0)) {
		return TransferJob{}, ErrInvalidTransfer
	}
	if err := validateAlias(input.Alias); err != nil {
		return TransferJob{}, err
	}
	if input.Direction == TransferRemote {
		if err := validateAlias(input.SourceAlias); err != nil {
			return TransferJob{}, err
		}
		if input.Operation != RemoteCopy && input.Operation != RemoteMove {
			return TransferJob{}, ErrInvalidTransfer
		}
		if _, err := cleanPublicPath(input.SourcePath, false); err != nil {
			return TransferJob{}, err
		}
	} else if input.SourceAlias != "" || input.SourcePath != "" || input.Operation != "" {
		return TransferJob{}, ErrInvalidTransfer
	}
	cleaned, err := cleanPublicPath(input.RemotePath, false)
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
	batchName := strings.TrimSpace(input.BatchName)
	if batchName == "" {
		batchName = name
	}
	batchKind := input.BatchKind
	if batchKind == "" {
		batchKind = input.Kind
	}
	if len(batchName) > 1024 || (batchKind != TransferFile && batchKind != TransferFolder) {
		return TransferJob{}, ErrInvalidTransfer
	}

	m.jobsMutex.Lock()
	m.initializeJobsLocked()
	staleRemotes := m.sweepJobsLocked()
	if existing := m.jobs[input.ID]; existing != nil {
		if sameTransferIdentity(existing.job, input, cleaned, name) {
			job := existing.job
			m.jobsMutex.Unlock()
			closeRemotes(staleRemotes)
			return job, nil
		}
		m.jobsMutex.Unlock()
		closeRemotes(staleRemotes)
		return TransferJob{}, ErrConflict
	}
	if len(m.jobOrder) >= maxRetainedTransferJobs {
		var cleanup *transferJobRecord
		removedWithoutNetwork := false
		// Prefer records which have no remote partial state. A temporarily
		// unreachable upload tombstone must not freeze admission while a completed
		// download can be discarded without network I/O.
		for index, id := range m.jobOrder {
			record := m.jobs[id]
			if record == nil || record.cleanupInFlight || !evictableTransferJob(record.job) ||
				(record.job.Direction == TransferUpload && record.job.Kind == TransferFile) {
				continue
			}
			delete(m.jobs, id)
			m.jobOrder = append(m.jobOrder[:index], m.jobOrder[index+1:]...)
			removedWithoutNetwork = true
			break
		}
		if !removedWithoutNetwork {
			for _, id := range m.jobOrder {
				record := m.jobs[id]
				if record != nil && !record.cleanupInFlight && record.job.Direction == TransferUpload &&
					record.job.Kind == TransferFile && record.job.Problem == "sftp_cleanup_pending" {
					cleanup = record
					break
				}
			}
		}
		if !removedWithoutNetwork && cleanup == nil {
			for index, id := range m.jobOrder {
				record := m.jobs[id]
				if record == nil || record.cleanupInFlight || !evictableTransferJob(record.job) {
					continue
				}
				if record.job.Direction == TransferUpload && record.job.Kind == TransferFile {
					cleanup = record
					break
				}
				delete(m.jobs, id)
				m.jobOrder = append(m.jobOrder[:index], m.jobOrder[index+1:]...)
				break
			}
		}
		if cleanup != nil {
			cleanup.cleanupInFlight = true
			cleanup.cleanupTombstone = true
			cleanup.job.Problem = "sftp_cleanup_pending"
			cleanup.job.UpdatedAt = m.now().UTC()
			cleanupJob := cleanup.job
			m.jobsMutex.Unlock()
			closeRemotes(staleRemotes)
			cleanupErr := m.cleanupEvictedUploadPart(cleanupJob)
			m.jobsMutex.Lock()
			if current := m.jobs[cleanupJob.ID]; current == cleanup {
				current.cleanupInFlight = false
				if cleanupErr == nil {
					delete(m.jobs, cleanupJob.ID)
					m.removeJobOrderLocked(cleanupJob.ID)
				}
			}
			m.jobsMutex.Unlock()
			if cleanupErr != nil {
				return TransferJob{}, ErrTransferLimit
			}
			return m.CreateJob(input)
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
	m.jobs[input.ID] = &transferJobRecord{job: job, sampleAt: now}
	m.jobOrder = append(m.jobOrder, input.ID)
	_ = m.persistJobsLocked(true)
	m.jobsMutex.Unlock()
	closeRemotes(staleRemotes)
	return job, nil
}

func (m *TransferManager) removeJobOrderLocked(id string) {
	for index, candidate := range m.jobOrder {
		if candidate == id {
			m.jobOrder = append(m.jobOrder[:index], m.jobOrder[index+1:]...)
			return
		}
	}
}

func (m *TransferManager) cleanupEvictedUploadPart(job TransferJob) error {
	m.releasePreparedDownload(job.ID)
	if job.Direction != TransferUpload || job.Kind != TransferFile || m.Service == nil || m.Service.Open == nil {
		return ErrTransferState
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return m.Cancel(ctx, job.Alias, job.ID, job.RemotePath)
}

func (m *TransferManager) ListJobs() []TransferJob {
	m.sweepPreparedDownloads()
	m.expireFinishedJobs()
	m.jobsMutex.Lock()
	m.initializeJobsLocked()
	staleRemotes := m.sweepJobsLocked()
	result := make([]TransferJob, 0, len(m.jobOrder))
	for _, id := range m.jobOrder {
		if record := m.jobs[id]; record != nil {
			result = append(result, record.job)
		}
	}
	_ = m.persistJobsLocked(false)
	m.jobsMutex.Unlock()
	closeRemotes(staleRemotes)
	return result
}

// ClearFinished removes terminal records from the engine-owned ledger. Active,
// paused and failed work remains available for control or diagnosis.
func (m *TransferManager) ClearFinished() int {
	m.jobsMutex.Lock()
	m.initializeJobsLocked()
	removed := make([]string, 0)
	kept := m.jobOrder[:0]
	for _, id := range m.jobOrder {
		record := m.jobs[id]
		if record != nil && (record.job.Status == TransferCompleted || record.job.Status == TransferCancelled) {
			delete(m.jobs, id)
			delete(m.dataPlane, id)
			removed = append(removed, id)
			continue
		}
		kept = append(kept, id)
	}
	m.jobOrder = kept
	_ = m.persistJobsLocked(true)
	m.jobsMutex.Unlock()
	for _, id := range removed {
		m.releasePreparedDownload(id)
	}
	return len(removed)
}

func (m *TransferManager) UpdateJob(id string, update UpdateTransferJob) (TransferJob, error) {
	if m.isClosed() {
		return TransferJob{}, ErrUnavailable
	}
	return m.updateJob(id, update, transferUpdateInternal)
}

// UpdateJobFromClient accepts queue controls but reserves upload progress and
// terminal publication state for the owned data-plane endpoints.
func (m *TransferManager) UpdateJobFromClient(id string, update UpdateTransferJob) (TransferJob, error) {
	if m.isClosed() {
		return TransferJob{}, ErrUnavailable
	}
	// The same operation lock is held by upload publication. A pause/fail cannot
	// change the queue state between the atomic rename and its terminal commit.
	unlock := m.lock("", "\x00job-owner:"+id)
	defer unlock()
	job, err := m.updateJob(id, update, transferUpdateClient)
	if err == nil && job.Direction == TransferDownload && (update.Action == TransferCancelAction || update.Action == TransferCompleteAction) {
		m.releasePreparedDownload(id)
	}
	if err == nil && job.Direction == TransferRemote {
		switch update.Action {
		case TransferPauseAction, TransferCancelAction:
			m.cancelRemoteJob(id)
		case TransferResumeAction, TransferRetryAction:
			m.ScheduleRemoteJob(id)
		}
	}
	return job, err
}

// updateUploadJob is reserved for the upload data plane. Keeping this entry
// point private makes the server, rather than a browser-supplied follow-up
// request, authoritative for upload progress and terminal state.
func (m *TransferManager) updateUploadJob(id string, update UpdateTransferJob) (TransferJob, error) {
	return m.updateJob(id, update, transferUpdateUploadData)
}

func (m *TransferManager) updateJob(id string, update UpdateTransferJob, origin transferUpdateOrigin) (TransferJob, error) {
	if !transferIDPattern.MatchString(id) {
		return TransferJob{}, ErrInvalidTransfer
	}
	m.jobsMutex.Lock()
	m.initializeJobsLocked()
	staleRemotes := m.sweepJobsLocked()
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
	if record.cleanupInFlight || (record.cleanupTombstone && !(origin == transferUpdateUploadData && update.Action == TransferCancelAction)) {
		return TransferJob{}, ErrTransferState
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
	if origin == transferUpdateUploadData && job.Direction != TransferUpload {
		return TransferJob{}, ErrConflict
	}
	if err := validateTransferUpdate(*job, update, origin); err != nil {
		return TransferJob{}, err
	}
	now := m.now().UTC()

	switch update.Action {
	case TransferStartAction:
		if job.Status != TransferQueued {
			return TransferJob{}, ErrTransferState
		}
		// 停止中は待機のまま置く。実行中のものは止めない。止めたければ
		// pause がある。
		if m.processingStopped {
			return TransferJob{}, ErrTransferLimit
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
	case TransferRetryAction:
		if job.Status != TransferFailed {
			return TransferJob{}, ErrTransferState
		}
		if update.ResetProgress {
			job.TransferredBytes = 0
			if job.Direction == TransferDownload {
				record.sentBytes, record.revision = 0, ""
				job.DownloadRevision = ""
			}
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
			committed = true
			return *job, nil
		}
		if update.TransferredBytes != nil {
			job.TransferredBytes = *update.TransferredBytes
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
	if job.Status != TransferRunning && job.Status != TransferQueued {
		terminalRemote = m.detachRemote(job.Alias, job.ID, job.RemotePath)
	}
	_ = m.persistJobsLocked(update.Action != TransferProgressAction)
	committed = true
	return *job, nil
}

// validateTransferUpdate is deliberately side-effect free. Browser fields are
// checked together with direction and state while jobsMutex is held, before a
// single byte or slot counter can be mutated.
func validateTransferUpdate(job TransferJob, update UpdateTransferJob, origin transferUpdateOrigin) error {
	hasTransferred := update.TransferredBytes != nil
	hasTotal := update.TotalBytes != nil
	hasProblem := strings.TrimSpace(update.Problem) != ""
	noPayload := !hasTransferred && !hasTotal && !hasProblem && !update.ResetProgress

	if origin == transferUpdateClient {
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
	case TransferStartAction, TransferPauseAction, TransferCancelAction, TransferNeedsOverwriteAction:
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

func (m *TransferManager) sweepJobsLocked() []Remote {
	now := m.now().UTC()
	var detached []Remote
	for _, record := range m.jobs {
		if m.dataPlane[record.job.ID] > 0 {
			continue
		}
		if record.job.Status == TransferRunning && now.Sub(record.job.UpdatedAt) > staleRunningTransferAfter {
			record.job.Status = TransferFailed
			record.job.Problem = "transfer_interrupted"
			record.job.UpdatedAt = now
			m.releaseJobLocked(TransferRunning)
			if remote := m.detachRemote(record.job.Alias, record.job.ID, record.job.RemotePath); remote != nil {
				detached = append(detached, remote)
			}
		}
	}
	return detached
}

// expireFinishedJobs は、完了と取消の記録を設定された時間で落とす。失敗は
// 残す。原因を見る前に消えてはならないからである。
//
// 解放は job の lock を手放してから行う。prepared download は別の mutex が
// 守っており、二つを重ねて取れば順序が閉じない。
func (m *TransferManager) expireFinishedJobs() {
	m.jobsMutex.Lock()
	m.initializeJobsLocked()
	if m.clearCompletedAfter <= 0 {
		m.jobsMutex.Unlock()
		return
	}
	now := m.now().UTC()
	removed := make([]string, 0)
	kept := m.jobOrder[:0]
	for _, id := range m.jobOrder {
		record := m.jobs[id]
		if record != nil && terminalTransferStatus(record.job.Status) &&
			m.dataPlane[id] == 0 && now.Sub(record.job.UpdatedAt) >= m.clearCompletedAfter {
			delete(m.jobs, id)
			delete(m.dataPlane, id)
			removed = append(removed, id)
			continue
		}
		kept = append(kept, id)
	}
	m.jobOrder = kept
	m.jobsMutex.Unlock()
	for _, id := range removed {
		m.releasePreparedDownload(id)
	}
}

func closeRemotes(remotes []Remote) {
	for _, remote := range remotes {
		_ = remote.Close()
	}
}

func (m *TransferManager) releaseJobLocked(status TransferJobStatus) {
	if status == TransferRunning && m.activeJobs > 0 {
		m.activeJobs--
	}
}

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
		job.BatchName == batchName && job.BatchKind == batchKind && job.LastModified == input.LastModified && job.Overwrite == input.Overwrite &&
		job.LargeFileThresholdBytes == input.LargeFileThresholdBytes && job.LargeFileParallelism == input.LargeFileParallelism &&
		job.LargeFileChunkBytes == input.LargeFileChunkBytes
}

func terminalTransferStatus(status TransferJobStatus) bool {
	return status == TransferCompleted || status == TransferCancelled
}

func retainedTransferStatus(status TransferJobStatus) bool {
	return terminalTransferStatus(status) || status == TransferFailed
}

func evictableTransferJob(job TransferJob) bool {
	return retainedTransferStatus(job.Status) && !(job.Direction == TransferUpload && job.Problem == "sftp_cleanup_pending")
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
