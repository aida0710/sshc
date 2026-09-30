package application

import (
	"sshc/internal/config"
	"sshc/internal/secret"
	"sshc/internal/storage"
)

// StartupRemover prepares, under the vault's stable generation, the encrypted
// change that drops the startup snippet assignments of removed aliases. The
// callback commits it together with the SSH configuration.
type StartupRemover interface {
	WithStartupRemoval(aliases []string, commit func(*storage.Change) (storage.Result, error)) (storage.Result, error)
}

func (s *Service) SetStartupRemover(remover StartupRemover) {
	s.startupRemover = remover
}

// saveAliasRemoval keeps a deleted host's startup snippet from reaching a host
// created later under the same alias, which never opted in (design.md: startup
// snippets are an opt-in per alias). The Connections screen deletes a host with
// a file_raw save, so the removal is read from the edit itself.
func (s *Service) saveAliasRemoval(vault *secret.Service, request EditRequest) (SaveResult, error) {
	removed, err := aliasesRemovedBy(request)
	if err != nil {
		return SaveResult{}, err
	}
	if len(removed) == 0 || s.startupRemover == nil {
		return s.Save(request)
	}
	var saved SaveResult
	err = vault.WithStableSnapshot(func() error {
		_, err := s.startupRemover.WithStartupRemoval(removed, func(startupChange *storage.Change) (storage.Result, error) {
			updated, committed, err := s.commitWithSealedChanges(request, []*storage.Change{startupChange})
			saved = updated
			return committed, err
		})
		return err
	})
	return saved, err
}

// aliasesRemovedBy returns the concrete aliases whose Host declaration a
// file_raw or block_raw save removes from the edited file. An alias the file
// still declares, even in another block, keeps its assignments.
func aliasesRemovedBy(request EditRequest) ([]string, error) {
	before := config.Parse([]byte(request.Base))
	var after *config.File
	switch request.Kind {
	case EditFileRaw:
		after = config.Parse([]byte(request.Raw))
	case EditBlockRaw:
		after = config.Parse([]byte(request.Base))
		block, found := FindHostBlock(after, request.Alias)
		if !found {
			return nil, ErrHostNotFound
		}
		if err := ReplaceBlock(after, block, request.Raw); err != nil {
			return nil, err
		}
	default:
		return nil, nil
	}
	remaining := make(map[string]bool)
	for _, alias := range declaredAliases(after.Lines) {
		remaining[alias] = true
	}
	var removed []string
	for _, alias := range declaredAliases(before.Lines) {
		if !remaining[alias] {
			removed = append(removed, alias)
		}
	}
	return removed, nil
}
