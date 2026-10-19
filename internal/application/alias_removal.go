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
// file_raw, block_raw or delete_hosts save removes from the edited file. An
// alias the file still declares, even in another block, keeps its assignments.
func aliasesRemovedBy(request EditRequest) ([]string, error) {
	if request.Kind == EditDeleteHosts {
		return aliasesRemovedByHostDeletes(request)
	}
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

// aliasesRemovedByHostDeletes は、delete_hosts がファイルごとに消す alias を返す。
// 消したあとも同じファイルのほかのブロックが宣言している alias は含めない。
func aliasesRemovedByHostDeletes(request EditRequest) ([]string, error) {
	deleted := make(map[string][]string)
	for _, host := range request.Hosts {
		deleted[host.Path] = append(deleted[host.Path], host.Alias)
	}
	var removed []string
	for _, base := range request.Bases {
		after := config.Parse([]byte(base.Base))
		for _, alias := range deleted[base.Path] {
			if _, err := ExtractHostBlock(after, alias); err != nil {
				return nil, err
			}
		}
		remaining := make(map[string]bool)
		for _, alias := range declaredAliases(after.Lines) {
			remaining[alias] = true
		}
		for _, alias := range declaredAliases(config.Parse([]byte(base.Base)).Lines) {
			if !remaining[alias] {
				removed = append(removed, alias)
			}
		}
	}
	return removed, nil
}
