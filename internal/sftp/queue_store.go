package sftp

import (
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"
	"sshc/internal/storage"
	"time"
)

const transferQueueSchemaVersion = 1

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
	contents, err := storage.ReadFileLimited(storage.OSFileSystem{}, filename, storage.MaxFileSize)
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
			if job.Status == TransferRunning {
				if job.Direction == TransferRemote {
					if job.Problem == RemoteReconciliationProblem {
						job.Status = TransferReattach
					} else {
						job.Status = TransferQueued
						job.TransferredBytes = 0
					}
				} else {
					job.Status = TransferReattach
				}
				job.BytesPerSecond, job.RemainingSeconds = 0, -1
				if job.Problem != RemoteReconciliationProblem {
					job.Problem = "transfer_interrupted"
				}
				job.UpdatedAt = m.now().UTC()
			}
			record := &transferJobRecord{job: job, sampleAt: m.now().UTC(), sampleBytes: job.TransferredBytes}
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
		m.ScheduleRemoteJob(id)
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
	if !transferIDPattern.MatchString(job.ID) || !transferIDPattern.MatchString(job.BatchID) ||
		(job.Direction != TransferUpload && job.Direction != TransferDownload && job.Direction != TransferRemote) ||
		(job.Kind != TransferFile && job.Kind != TransferFolder) || job.TotalBytes < -1 || job.TransferredBytes < 0 ||
		(job.TotalBytes >= 0 && job.TransferredBytes > job.TotalBytes) || job.Attempt < 1 || job.CreatedAt.IsZero() || job.UpdatedAt.IsZero() {
		return ErrInvalidTransfer
	}
	if err := validateAlias(job.Alias); err != nil {
		return err
	}
	var pathErr error
	if job.Operation == RemoteGet {
		_, pathErr = localRelative(job.RemotePath)
	} else {
		_, pathErr = cleanPublicPath(job.RemotePath, false)
	}
	if err := pathErr; err != nil {
		return err
	}
	if (job.LargeFileThresholdBytes != 0 &&
		(job.LargeFileThresholdBytes < MinLargeFileThreshold || job.LargeFileThresholdBytes > MaxLargeFileThreshold)) ||
		(job.LargeFileParallelism != 0 &&
			(job.LargeFileParallelism < 1 || job.LargeFileParallelism > MaxLargeFileParallelism)) ||
		(job.LargeFileChunkBytes != 0 &&
			(job.LargeFileChunkBytes < MinLargeFileChunkBytes || job.LargeFileChunkBytes > MaxLargeFileChunkBytes)) ||
		(((job.Direction != TransferDownload && job.Direction != TransferUpload) || job.Kind != TransferFile) &&
			(job.LargeFileThresholdBytes != 0 || job.LargeFileParallelism != 0 || job.LargeFileChunkBytes != 0)) {
		return ErrInvalidTransfer
	}
	if err := validateUploadRanges(job.UploadRanges, job.TotalBytes); err != nil ||
		(job.Direction != TransferUpload && len(job.UploadRanges) != 0) || len(job.UploadRanges) > 65536 ||
		(len(job.UploadRanges) != 0 && uploadRangeBytes(job.UploadRanges) != job.TransferredBytes) {
		return ErrInvalidTransfer
	}
	if job.Direction == TransferRemote {
		if err := validateAlias(job.SourceAlias); err != nil {
			return err
		}
		var sourceErr error
		if job.Operation == RemotePut {
			_, sourceErr = localRelative(job.SourcePath)
		} else {
			_, sourceErr = cleanPublicPath(job.SourcePath, false)
		}
		if err := sourceErr; err != nil {
			return err
		}
		if job.Operation != RemoteCopy && job.Operation != RemoteMove && job.Operation != RemoteDelete && job.Operation != RemoteGet && job.Operation != RemotePut {
			return ErrInvalidTransfer
		}
		if (job.Operation == RemoteGet || job.Operation == RemotePut) && job.SourceAlias != job.Alias {
			return ErrInvalidTransfer
		}
		if job.Operation == RemoteDelete && (job.SourceAlias != job.Alias || job.SourcePath != job.RemotePath || job.Overwrite) {
			return ErrInvalidTransfer
		}
	} else if job.SourceAlias != "" || job.SourcePath != "" || job.Operation != "" {
		return ErrInvalidTransfer
	}
	switch job.Status {
	case TransferQueued, TransferRunning, TransferPaused, TransferReattach, TransferNeedsOverwrite, TransferCompleted, TransferFailed, TransferCancelled:
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
	if !force && !m.lastQueuePersist.IsZero() && now.Sub(m.lastQueuePersist) < time.Second {
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
	fileSystem := storage.OSFileSystem{}
	if err := fileSystem.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		return err
	}
	return storage.WriteAtomicFile(fileSystem, filename, ".sshc-"+filepath.Base(filename)+".tmp-", storage.FilePermission, contents)
}
