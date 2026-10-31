package application

import "sshc/internal/storage"

// metadata.json を書く道。設定・接続・VPN などの保存は、ここを通して metadata 全体を1つの変更として
// 書く。読んだときの前提（precondition）と違えば、storage が書かずに断る。

// changeMetadata は、ほかの保存と順に、今の metadata を読んで change で変え、operation として書く。
// change が error を返したら書かない。画面が読んだときの値（base）と比べて断るのは change の役目。
func (s *Service) changeMetadata(operation string, change func(stored *Metadata) error) (SaveResult, error) {
	s.saveMutex.Lock()
	defer s.saveMutex.Unlock()
	stored, precondition, err := s.metadata.Load()
	if err != nil {
		return SaveResult{}, err
	}
	if err := change(&stored); err != nil {
		return SaveResult{}, err
	}
	return s.commitMetadata(stored, precondition, operation)
}

// commitMetadata は、metadata だけを書く。
func (s *Service) commitMetadata(stored Metadata, precondition storage.Precondition, operation string) (SaveResult, error) {
	return s.commitMetadataWith(metadataCommit{operation: operation, metadata: stored, precondition: precondition}, nil)
}

// metadataCommit は、書く前の metadata と、読んだときの前提である。
type metadataCommit struct {
	operation    string
	metadata     Metadata
	precondition storage.Precondition
}

// commitMetadataWith は、metadata と、あれば別の変更（vault など）を、ひとつの
// storage.Request で書く。どちらかだけが書かれることはない。
//
// 別の変更があれば、接続の作成と同じく CommitAtomic で書き、書き込みの途中で失敗
// したときに両方をその場で巻き戻す。metadata だけを書いて vault が古いまま残ると、
// 改名したプロファイルのシークレットが見つからず、保留の記録がほかの保存も止める。
func (s *Service) commitMetadataWith(planned metadataCommit, alongside *storage.Change) (SaveResult, error) {
	if err := s.metadata.EnsureDirectory(); err != nil {
		return SaveResult{}, err
	}
	change, err := s.metadata.Change(planned.metadata, planned.precondition)
	if err != nil {
		return SaveResult{}, err
	}
	request := storage.Request{Operation: planned.operation, Changes: []storage.Change{change}}
	commit := s.manager.Commit
	if alongside != nil {
		request.Changes = append(request.Changes, *alongside)
		commit = s.manager.CommitAtomic
	}
	result, err := commit(request)
	if err != nil {
		return SaveResult{}, err
	}
	return SaveResult{TransactionID: result.ID, Written: result.Written}, nil
}
