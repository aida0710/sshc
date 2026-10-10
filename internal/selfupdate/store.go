package selfupdate

import (
	"encoding/json"
	"errors"
	"os"
	"time"

	"sshc/internal/filelock"
	"sshc/internal/storage"
	"sshc/internal/strictjson"
)

// maximumJobDocumentSize bounds the one plan and prevents an oversized local file from consuming memory.
const maximumJobDocumentSize = 16 << 10

// stateLockTimeout bounds a short atomic read/modify/write shared with the restart helper.
const stateLockTimeout = time.Second

// The workspace already excludes .sshc- temporary files from remote sync.
const temporaryJobPrefix = ".sshc-web-update-"

// Store persists only one job, with no installer output or authentication values.
type Store struct{ Path string }

func (store Store) Change(change func(*Job) error) (Job, error) {
	release, err := filelock.AcquireWithin(store.Path+StateLockSuffix, stateLockTimeout)
	if err != nil {
		return Job{}, ErrState
	}
	defer release()
	job, err := store.Read()
	if err != nil {
		return Job{}, ErrState
	}
	if err := change(&job); err != nil {
		return job, err
	}
	body, err := json.Marshal(job)
	if err != nil {
		return Job{}, ErrState
	}
	if err := storage.WriteAtomicFile(storage.OSFileSystem{}, store.Path, temporaryJobPrefix, storage.FilePermission, body); err != nil {
		return Job{}, ErrState
	}
	return job, nil
}

func (store Store) Read() (Job, error) {
	body, err := storage.ReadFileLimited(storage.OSFileSystem{}, store.Path, maximumJobDocumentSize)
	if errors.Is(err, os.ErrNotExist) {
		return Job{}, nil
	}
	if err != nil {
		return Job{}, ErrState
	}
	var job Job
	if err := strictjson.Decode(body, &job); err != nil {
		return Job{}, ErrState
	}
	switch job.State {
	case JobAccepted, JobInstalling, JobRestarting, JobSucceeded, JobFailed, JobRestartRequired:
	default:
		return Job{}, ErrState
	}
	if len(job.ID) != jobIDLength || job.Target != job.Plan.Target || job.OwnerPID <= 0 {
		return Job{}, ErrState
	}
	if job.InstalledVersion != "" && validateInstalledVersion(job.Plan, job.InstalledVersion) != nil {
		return Job{}, ErrState
	}
	return job, nil
}
