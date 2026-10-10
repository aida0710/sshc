package sftp

import (
	"errors"
	"io/fs"
	"path/filepath"
)

// Resolve the existing prefix even when a queued get will create the rest.
func localTransferPath(value string) (string, error) {
	cleaned, err := cleanLocalPath(value)
	if err != nil {
		return "", err
	}
	parent := filepath.Dir(filepath.FromSlash(cleaned))
	suffix := filepath.Base(filepath.FromSlash(cleaned))
	for {
		resolved, err := filepath.EvalSymlinks(parent)
		if err == nil {
			return filepath.ToSlash(filepath.Join(resolved, suffix)), nil
		}
		if !errors.Is(err, fs.ErrNotExist) || filepath.Dir(parent) == parent {
			return "", err
		}
		suffix = filepath.Join(filepath.Base(parent), suffix)
		parent = filepath.Dir(parent)
	}
}

func (m *TransferManager) refuseLocalTransferOverlap(targets []string) error {
	protectedPaths := make(map[string]struct{})
	m.remoteJobsMutex.Lock()
	running := make(map[string]bool, len(m.remoteRuns))
	for id := range m.remoteRuns {
		running[id] = true
	}
	for _, localPath := range m.localTransferRuns {
		protectedPaths[localPath] = struct{}{}
	}
	m.remoteJobsMutex.Unlock()
	m.jobsMutex.Lock()
	for id, record := range m.jobs {
		job := record.job
		if terminalTransferStatus(job.Status) && !running[id] {
			continue
		}
		if localPath := localPathForTransfer(job); localPath != "" {
			protectedPaths[localPath] = struct{}{}
		}
	}
	m.jobsMutex.Unlock()
	for localPath := range protectedPaths {
		resolved, err := localTransferPath(localPath)
		if err != nil {
			return ErrConflict
		}
		for _, target := range targets {
			if localPathsOverlap(target, resolved) {
				return ErrConflict
			}
		}
	}
	return nil
}

func localPathForTransfer(job TransferJob) string {
	switch job.Operation {
	case RemotePut:
		return job.SourcePath
	case RemoteGet:
		return job.RemotePath
	default:
		return ""
	}
}

func (m *TransferManager) protectLocalTransferRun(run *remoteRun, job TransferJob) error {
	localPath := localPathForTransfer(job)
	if localPath == "" {
		return nil
	}
	// Registration precedes filesystem access and shares the mutation lock.
	// A job removed while waiting must not acquire its path after deletion.
	m.localMutationsMutex.Lock()
	defer m.localMutationsMutex.Unlock()
	if err := run.ownershipError(); err != nil {
		return err
	}
	m.remoteJobsMutex.Lock()
	m.localTransferRuns[run] = localPath
	m.remoteJobsMutex.Unlock()
	return nil
}
