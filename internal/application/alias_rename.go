package application

import (
	"sshc/internal/secret"
	"sshc/internal/storage"
)

// StartupRenamer prepares an encrypted binding change under the vault's stable
// generation. The callback commits it together with the SSH configuration.
type StartupRenamer interface {
	WithStartupRename(from, to string, commit func(*storage.Change) (storage.Result, error)) (storage.Result, error)
}

func (s *Service) SetStartupRenamer(renamer StartupRenamer) {
	s.startupRenamer = renamer
}

// SaveWithSecrets owns the whole alias transition. A locked vault retains the
// existing contract: config and public metadata can still be renamed alone.
func (s *Service) SaveWithSecrets(secrets *secret.Service, request EditRequest) (SaveResult, error) {
	if request.Kind != EditRename || secrets == nil || !secrets.Unlocked() {
		return s.Save(request)
	}
	var saved SaveResult
	_, err := secrets.WithConnectionSecretsTransaction(secret.ConnectionSecretsMutation{
		Rename: &secret.AliasRename{From: request.Alias, To: request.NewAlias},
	}, func(vaultChange *storage.Change) (storage.Result, error) {
		commit := func(startupChange *storage.Change) (storage.Result, error) {
			updated, committed, err := s.commitAliasRename(request, []*storage.Change{vaultChange, startupChange})
			saved = updated
			return committed, err
		}
		if s.startupRenamer != nil {
			return s.startupRenamer.WithStartupRename(request.Alias, request.NewAlias, commit)
		}
		return commit(nil)
	})
	return saved, err
}

func (s *Service) commitAliasRename(request EditRequest, changes []*storage.Change) (SaveResult, storage.Result, error) {
	s.saveMutex.Lock()
	defer s.saveMutex.Unlock()
	prepared, err := s.plan(request)
	if err != nil {
		return SaveResult{}, storage.Result{}, err
	}
	storageRequest := s.requestFor(prepared)
	storageRequest.Directories = append(storageRequest.Directories, storage.DirectoryCreate{Path: s.workspace.StateDir()})
	for _, change := range changes {
		if change != nil {
			storageRequest.Changes = append(storageRequest.Changes, *change)
		}
	}
	committed, err := s.commitAtomicPlannedRequest(prepared, storageRequest)
	if err != nil {
		return SaveResult{}, committed, err
	}
	return s.connectionUpdateResult(committed, prepared), committed, nil
}
