package application

import (
	"errors"
	"slices"

	"sshc/internal/storage"
)

// ErrKeyFilesChanged は、パスフレーズの割り当てを移す準備のあいだに、移す鍵ファイルの
// 組が変わったことを報告する。何も書いていないので、読み直してやり直せばよい。
var ErrKeyFilesChanged = errors.New("key files changed while the relocation was being prepared")

// ErrKeyPassphraseVaultMissing は、鍵のパスを変える操作で、パスフレーズの割り当てを移す
// Vault が SetVault で渡されていないことを表す。Vault のファイルが無い構成は
// secret.Service が扱うので、nil になるのは配線が外れたときだけである。nil を
// 「移す割り当てが無い」と読むと、割り当てを旧パスに残したまま成功を返す。
var ErrKeyPassphraseVaultMissing = errors.New("key relocation has no vault to move passphrase assignments")

// commitRelocatingKeyPassphrases は、鍵ファイルを動かす計画を、名前付きパスフレーズの
// 割り当てと専用パスフレーズを新しいパスへ移す Vault の変更と、1 つの storage
// トランザクションで確定する。鍵のパス変更（グループの改名・削除と鍵の移動）は、
// どれもここを通る。
//
// Vault の書き手の錠は saveMutex より先に取る決まりである（SaveWithSecrets と
// UpdateConnection も同じ順）。そのため、移す鍵を知るために錠の外で一度計画し、
// 錠を取ってから計画し直す。2 回の計画で移す鍵がずれたら、そのあいだに鍵が
// 動いたので、何も書かずに ErrKeyFilesChanged で断る。
//
// Vault を書くときは CommitAtomic を使う。途中で失敗したら鍵ファイルの移動ごと
// 巻き戻し、メモリ上の Vault とディスクを食い違わせない。Vault を書かないとき
// （移す鍵が無い、Vault のファイルが無い）は、これまでどおり Commit で確定する。
func (s *Service) commitRelocatingKeyPassphrases(plan func() (planned, error)) (planned, storage.Result, error) {
	s.saveMutex.Lock()
	prepared, err := plan()
	if err != nil || len(prepared.keyRelocations) == 0 || s.vault == nil {
		defer s.saveMutex.Unlock()
		if err != nil {
			return planned{}, storage.Result{}, err
		}
		if len(prepared.keyRelocations) != 0 {
			return planned{}, storage.Result{}, ErrKeyPassphraseVaultMissing
		}
		result, err := s.commitPlannedRequest(prepared, s.requestFor(prepared))
		return prepared, result, err
	}
	relocations := prepared.keyRelocations
	s.saveMutex.Unlock()

	var committed planned
	result, err := s.vault.WithKeyPassphraseRelocation(keyRelocationMap(relocations),
		func(vaultChange *storage.Change) (storage.Result, error) {
			s.saveMutex.Lock()
			defer s.saveMutex.Unlock()
			replanned, err := plan()
			if err != nil {
				return storage.Result{}, err
			}
			if !slices.Equal(replanned.keyRelocations, relocations) {
				return storage.Result{}, ErrKeyFilesChanged
			}
			committed = replanned
			request := s.requestFor(replanned)
			if vaultChange == nil {
				return s.commitPlannedRequest(replanned, request)
			}
			request.Changes = append(request.Changes, *vaultChange)
			return s.commitAtomicPlannedRequest(replanned, request)
		})
	if err != nil {
		return planned{}, storage.Result{}, err
	}
	return committed, result, nil
}

func keyRelocationMap(files []RelocatedKeyFile) map[string]string {
	relocations := make(map[string]string, len(files))
	for _, file := range files {
		relocations[file.From] = file.To
	}
	return relocations
}
