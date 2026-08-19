package application

import (
	"errors"
	"path/filepath"

	"sshc/internal/storage"
)

// commitPlannedRequest は、計画されたトランザクションを唯一のコミット経路に通す。
//
// **経路がひとつなのは、衝突の扱いが揃っていなければならないからである。** 以前は
// Save・commitGroupPlan・RelocateKey・接続の作成が同じ 20 行をそれぞれ書いており、
// 4 つとも少しずつ違っていた——このうち 3 つは、衝突したパスが自分の計画した設定
// ファイルでないときも三者マージの報告を組み立てており、base に nil を渡していた。
// 衝突したのが設定でないなら、報告できることは何も無いので、storage の答えをその
// まま返すのが正しい。

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
