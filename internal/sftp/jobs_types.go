package sftp

import "time"

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
	TransferReconnecting   TransferJobStatus = "reconnecting"
	TransferPaused         TransferJobStatus = "paused"
	TransferReattach       TransferJobStatus = "reattach"
	TransferNeedsOverwrite TransferJobStatus = "needs_overwrite"
	TransferCompleted      TransferJobStatus = "completed"
	TransferFailed         TransferJobStatus = "failed"
	TransferCancelled      TransferJobStatus = "cancelled"
)

// RemoteReconciliationProblem means the remote copy or move crossed its
// external commit point, but the device-local queue could not durably record
// the terminal result. It deliberately requires inspection/cancellation
// instead of automatically repeating a possibly completed move.
const RemoteReconciliationProblem = "sftp_reconciliation_required"

// CleanupPendingProblem marks a stopped job whose remote part file may still
// need removing. Eviction never drops such a job outright and, when room is
// needed, retries its cleanup first, so the value is state, not only a message.
const CleanupPendingProblem = "sftp_cleanup_pending"

// DownloadPartProgress describes one SFTP connection used while the engine
// prepares a large download. It is deliberately ephemeral: a prepared spool
// can be reused after completion, but in-flight connection progress cannot be
// resumed after an engine restart.
type DownloadPartProgress struct {
	Index            int   `json:"-"`
	TransferredBytes int64 `json:"-"`
	TotalBytes       int64 `json:"-"`
}

type TransferJob struct {
	ID               string
	BatchID          string
	BatchName        string
	BatchKind        TransferKind
	Alias            string
	SourceAlias      string
	SourcePath       string
	Operation        RemoteTransferOperation
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
	ReconnectAttempt int
	ReconnectAt      time.Time
	// Server file checkpoint is device-local and never supplied by a client.
	RemoteCheckpoint        *remoteFileCheckpoint `json:",omitempty"`
	Problem                 string
	LastModified            int64
	ExpectedRevision        string
	SourceFingerprint       string
	Overwrite               bool
	DownloadRevision        string
	LargeFileThresholdBytes int64
	LargeFileParallelism    int
	LargeFileChunkBytes     int64
	ExcludePatterns         []string
	DownloadParts           []DownloadPartProgress `json:"-"`
	UploadRanges            []UploadRange
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
	// uploadParallelism is how many connections the last StartOwned in this
	// process prepared the upload's part for: 1 for one stream, more for
	// ranges, 0 before any. Ranges and the completion check follow it rather
	// than the host's connection limit, which can change while the job runs.
	uploadParallelism int
}

func terminalTransferStatus(status TransferJobStatus) bool {
	return status == TransferCompleted || status == TransferCancelled
}

func retainedTransferStatus(status TransferJobStatus) bool {
	return terminalTransferStatus(status) || status == TransferFailed
}
