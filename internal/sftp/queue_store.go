package sftp

import (
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"sshc/internal/storage"
)

const (
	transferQueueSchemaVersion = 1
	// A queue holds up to 200 jobs, including paths and resumable ranges. Use
	// one bound for both directions so every accepted snapshot can be restored.
	maxTransferQueueBytes = 32 << 20
	// maxPersistedUploadRanges bounds the upload ranges one restored job may
	// carry. Finished chunks merge with the ranges next to them
	// (addUploadRange), so an upload keeps one range per run of finished
	// chunks, not one per chunk. The bound keeps the sorting and scanning each
	// later chunk does over the ranges small even for a damaged queue file.
	maxPersistedUploadRanges = 65536
	// queuePersistInterval is how often progress alone rewrites the queue.
	// Progress arrives many times a second and each write replaces the whole
	// queue file, so a restart loses at most the progress of the last
	// interval. Every other change is written at once (force).
	queuePersistInterval = time.Second
)

type persistedTransferQueue struct {
	SchemaVersion int           `json:"schemaVersion"`
	Jobs          []TransferJob `json:"jobs"`
}

// EnableQueuePersistence restores the device-local queue and enables atomic
// snapshots after mutations. The file deliberately lives below .ssh/sshc and
// is excluded from workspace sync.
func (m *TransferManager) EnableQueuePersistence(filename string) error {
	if m == nil || filename == "" {
		return ErrInvalidTransfer
	}
	contents, err := storage.ReadFileLimited(storage.OSFileSystem{}, filename, maxTransferQueueBytes)
	if errors.Is(err, storage.ErrFileTooLarge) {
		if preserveErr := preserveOversizedQueue(filename); preserveErr != nil {
			return preserveErr
		}
		// Keep oversized files from older writers for diagnosis, while letting
		// the SSH engine start with a fresh device-local queue.
		err = fs.ErrNotExist
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	var stored persistedTransferQueue
	if err == nil {
		invalid := json.Unmarshal(contents, &stored) != nil ||
			stored.SchemaVersion != transferQueueSchemaVersion ||
			len(stored.Jobs) > maxRetainedTransferJobs
		seen := make(map[string]struct{}, len(stored.Jobs))
		for _, job := range stored.Jobs {
			if validationErr := validPersistedJob(job); validationErr != nil {
				invalid = true
				break
			}
			if _, duplicate := seen[job.ID]; duplicate {
				invalid = true
				break
			}
			seen[job.ID] = struct{}{}
		}
		if invalid {
			// A damaged device-local queue must not make the whole SSH engine
			// unavailable. Preserve the exact bytes for diagnosis before replacing
			// the active queue with a valid empty snapshot.
			if preserveErr := preserveCorruptQueue(filename, contents); preserveErr != nil {
				return errors.Join(ErrInvalidTransfer, preserveErr)
			}
			stored = persistedTransferQueue{}
		}
	}
	m.jobsMutex.Lock()
	m.initializeJobsLocked()
	m.queuePath = filename
	if err == nil {
		for _, job := range stored.Jobs {
			if m.jobs[job.ID] != nil {
				m.jobsMutex.Unlock()
				return ErrInvalidTransfer
			}
			if job.Status == TransferRunning || job.Status == TransferReconnecting {
				if job.Direction == TransferRemote {
					if job.Status == TransferReconnecting {
						// Shutdown ends the recovery budget. Preserve the verified
						// checkpoint for an explicit resume after engine restart.
						job.Status = TransferPaused
					} else if job.Problem == RemoteReconciliationProblem {
						job.Status = TransferReattach
					} else {
						job.Status = TransferQueued
						job.TransferredBytes = 0
					}
				} else {
					job.Status = TransferReattach
				}
				job.BytesPerSecond, job.RemainingSeconds = 0, -1
				job.ReconnectAt = time.Time{}
				if job.Problem != RemoteReconciliationProblem {
					job.Problem = "transfer_interrupted"
				}
				job.UpdatedAt = m.now().UTC()
			}
			record := &transferJobRecord{job: job, sampleAt: m.now().UTC(), sampleBytes: job.TransferredBytes}
			if job.Direction == TransferDownload && job.DownloadRevision != "" {
				// A download's TransferredBytes only advances by an ACK that the engine
				// checked against the bytes it had sent, so it is also proof of what was
				// sent. Restoring both lets the browser resume from its checkpoint with
				// a Range request instead of starting over after an engine restart.
				record.revision = job.DownloadRevision
				record.sentBytes = job.TransferredBytes
			}
			m.jobs[job.ID] = record
			m.jobOrder = append(m.jobOrder, job.ID)
		}
	}
	m.activeJobs = 0
	if err := m.persistJobsLocked(true); err != nil {
		m.jobsMutex.Unlock()
		return err
	}
	remoteJobs := make([]string, 0)
	for _, id := range m.jobOrder {
		if job := m.jobs[id]; job != nil && job.job.Direction == TransferRemote && job.job.Status == TransferQueued {
			remoteJobs = append(remoteJobs, id)
		}
	}
	m.jobsMutex.Unlock()
	for _, id := range remoteJobs {
		m.scheduleRemoteJob(id)
	}
	return nil
}

// preserveCorruptQueue は壊れた queue を同じ directory の診断用 file へ退避する。
// The .sshc- prefix is also part of Remote Sync's denylist, keeping this
// device-local diagnostic snapshot from travelling to another machine.
func preserveCorruptQueue(filename string, contents []byte) error {
	fileSystem := storage.OSFileSystem{}
	directory := filepath.Dir(filename)
	if err := fileSystem.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	if _, err := fileSystem.WriteTemp(directory, ".sshc-"+filepath.Base(filename)+".corrupt-", storage.FilePermission, contents); err != nil {
		return err
	}
	return fileSystem.SyncDir(directory)
}

func validPersistedJob(job TransferJob) error {
	if _, err := validateTransferJobShape(job.shape()); err != nil {
		return err
	}
	if job.RemoteCheckpoint != nil {
		checkpoint := job.RemoteCheckpoint
		if !recoverableRemoteFile(job) || !strings.HasPrefix(checkpoint.SourceRevision, metadataRevisionPrefix) || !strings.HasPrefix(checkpoint.SourceContentRevision, contentRevisionPrefix) || (checkpoint.TargetRevision != AbsentRevision && !strings.HasPrefix(checkpoint.TargetRevision, metadataRevisionPrefix)) {
			return ErrInvalidTransfer
		}
	}
	if !validReconnectAttempts(job.ReconnectAttempt) {
		return ErrInvalidTransfer
	}
	if job.TransferredBytes < 0 || (job.TotalBytes >= 0 && job.TransferredBytes > job.TotalBytes) ||
		job.Attempt < 1 || job.CreatedAt.IsZero() || job.UpdatedAt.IsZero() {
		return ErrInvalidTransfer
	}
	if err := validateUploadRanges(job.UploadRanges, job.TotalBytes); err != nil ||
		(job.Direction != TransferUpload && len(job.UploadRanges) != 0) || len(job.UploadRanges) > maxPersistedUploadRanges ||
		(len(job.UploadRanges) != 0 && uploadRangeBytes(job.UploadRanges) != job.TransferredBytes) {
		return ErrInvalidTransfer
	}
	switch job.Status {
	case TransferQueued, TransferRunning, TransferReconnecting, TransferPaused, TransferReattach, TransferNeedsOverwrite, TransferCompleted, TransferFailed, TransferCancelled:
		return nil
	default:
		return ErrInvalidTransfer
	}
}

func (m *TransferManager) persistJobsLocked(force bool) error {
	if m.queuePath == "" {
		return nil
	}
	now := m.now().UTC()
	if !force && !m.lastQueuePersist.IsZero() && now.Sub(m.lastQueuePersist) < queuePersistInterval {
		return nil
	}
	jobs := make([]TransferJob, 0, len(m.jobOrder))
	for _, id := range m.jobOrder {
		if record := m.jobs[id]; record != nil {
			jobs = append(jobs, record.job)
		}
	}
	contents, err := json.Marshal(persistedTransferQueue{SchemaVersion: transferQueueSchemaVersion, Jobs: jobs})
	if err == nil {
		err = writeQueueAtomically(m.queuePath, contents)
	}
	m.queuePersistError = err
	if err == nil {
		m.lastQueuePersist = now
	}
	return err
}

// writeQueueAtomically は他の状態 file と同じ storage の経路で書く。symlink を辿らず、
// Windows では所有者だけの ACL と write-through の置換になる。
func writeQueueAtomically(filename string, contents []byte) error {
	if len(contents) > maxTransferQueueBytes {
		return storage.ErrFileTooLarge
	}
	fileSystem := storage.OSFileSystem{}
	if err := fileSystem.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		return err
	}
	return storage.WriteAtomicFile(fileSystem, filename, ".sshc-"+filepath.Base(filename)+".tmp-", storage.FilePermission, contents)
}

// Moving avoids reading an unbounded legacy queue into memory.
func preserveOversizedQueue(filename string) error {
	fileSystem := storage.OSFileSystem{}
	directory := filepath.Dir(filename)
	preserved, err := fileSystem.WriteTemp(directory, ".sshc-"+filepath.Base(filename)+".corrupt-", storage.FilePermission, nil)
	if err != nil {
		return err
	}
	if err := fileSystem.MovePrivate(filename, preserved); err != nil {
		_ = fileSystem.Remove(preserved)
		return err
	}
	return fileSystem.SyncDir(directory)
}
