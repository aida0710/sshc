package application

import (
	"sshc/internal/storage"
)

// SetKeepEngineRunning は、その選択を書き戻す。
//
// 他の設定と同じトランザクションマネージャを通す。**metadata を書く場所を
// 増やさない**——世代バックアップも競合の検出も、そこにしか無い。
func (s *Service) SetKeepEngineRunning(keep bool) (SaveResult, error) {
	stored, precondition, err := s.metadata.Load()
	if err != nil {
		return SaveResult{}, err
	}
	if stored.Desktop == nil {
		stored.Desktop = &Desktop{}
	}
	stored.Desktop.KeepRunning = keep

	if err := s.metadata.EnsureDirectory(); err != nil {
		return SaveResult{}, err
	}
	change, err := s.metadata.Change(stored, precondition)
	if err != nil {
		return SaveResult{}, err
	}
	result, err := s.manager.Commit(storage.Request{
		Operation: "desktop.keepRunning",
		Changes:   []storage.Change{change},
	})
	if err != nil {
		return SaveResult{}, err
	}
	return SaveResult{TransactionID: result.ID, Written: result.Written}, nil
}
