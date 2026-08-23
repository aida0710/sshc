package application

import (
	"errors"
	"path/filepath"

	"sshc/internal/storage"
)

// commitPlannedRequest は計画済みトランザクションを共通のコミット処理へ渡す。

func (s *Service) commitPlannedRequest(prepared planned, request storage.Request) (storage.Result, error) {
	return s.commitPlannedRequestWith(prepared, request, s.manager.Commit)
}

func (s *Service) commitAtomicPlannedRequest(prepared planned, request storage.Request) (storage.Result, error) {
	return s.commitPlannedRequestWith(prepared, request, s.manager.CommitAtomic)
}

func (s *Service) commitPlannedRequestWith(
	prepared planned,
	request storage.Request,
	commit func(storage.Request) (storage.Result, error),
) (storage.Result, error) {
	s.pendingBase = prepared.base
	s.pendingBaseline = prepared.baseline
	defer func() { s.pendingBase, s.pendingBaseline = nil, nil }()

	result, err := commit(request)
	var conflict *storage.ConflictError
	if errors.As(err, &conflict) {
		cleaned := filepath.Clean(conflict.Path)
		base, isConfiguration := prepared.base[cleaned]
		if !isConfiguration {
			return storage.Result{}, err
		}
		var edited []byte
		for _, change := range prepared.changes {
			if filepath.Clean(change.Path) == cleaned {
				edited = change.Contents
				break
			}
		}
		return storage.Result{}, &ConflictError{Report: BuildConflictReport(
			s.displayPath(cleaned), base, conflict.Current, edited,
		)}
	}
	return result, err
}
