package sftp

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
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
	contents, err := os.ReadFile(filename)
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

func preserveCorruptQueue(filename string, contents []byte) error {
	directory := filepath.Dir(filename)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	// The .sshc- prefix is also part of Remote Sync's denylist, keeping this
	// device-local diagnostic snapshot from travelling to another machine.
	backup, err := os.CreateTemp(directory, ".sshc-"+filepath.Base(filename)+".corrupt-*")
	if err != nil {
		return err
	}
	name := backup.Name()
	keep := false
	defer func() {
		_ = backup.Close()
		if !keep {
			_ = os.Remove(name)
		}
	}()
	if err := backup.Chmod(0o600); err != nil {
		return err
	}
	if _, err := backup.ReadFrom(bytes.NewReader(contents)); err != nil {
		return err
	}
	if err := backup.Sync(); err != nil {
		return err
	}
	if err := backup.Close(); err != nil {
		return err
	}
	keep = true
	if opened, err := os.Open(directory); err == nil {
		_ = opened.Sync()
		_ = opened.Close()
	}
	return nil
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
	if _, err := cleanPublicPath(job.RemotePath, false); err != nil {
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
		if _, err := cleanPublicPath(job.SourcePath, false); err != nil {
			return err
		}
		if job.Operation != RemoteCopy && job.Operation != RemoteMove {
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

func writeQueueAtomically(filename string, contents []byte) error {
	directory := filepath.Dir(filename)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".transfers-*.tmp")
	if err != nil {
		return err
	}
	name := temporary.Name()
	cleanup := true
	defer func() {
		_ = temporary.Close()
		if cleanup {
			_ = os.Remove(name)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, filename); err != nil {
		return err
	}
	cleanup = false
	if opened, err := os.Open(directory); err == nil {
		_ = opened.Sync()
		_ = opened.Close()
	}
	return nil
}
