package secret

import (
	"slices"

	"sshc/internal/storage"
)

// vaultTransaction は、vault の写しへの変更を、呼び手の storage トランザクションと
// 同じ書き込みに載せるための指定である。
//
// 接続の保存と VPN プロファイルの保存は、どちらも「設定ファイルと vault を 1 回で
// 書き、成功したときだけメモリ上の vault を差し替える」手順を踏む。手順そのものは
// ここに 1 か所だけ置き、変更の中身だけを呼び手が渡す。
type vaultTransaction struct {
	// apply は、写し（clone）へ変更を加え、変わったかを返す。current は解錠中の
	// vault そのもので、変わったかを比べるために読むだけである。
	apply func(current, clone *Vault) (bool, error)
	// commitsWithoutVault は、vault がまだ作られていないときに、vault を書かずに
	// commit へ進めてよいかである。消すだけの変更は、消す相手が無いので進めてよい。
	commitsWithoutVault bool
	// commit は、呼び手の storage トランザクションを書く。vault を書き換えないときは
	// nil を受け取る。
	commit func(vaultChange *storage.Change) (storage.Result, error)
}

// commitVaultTransaction は、vault の書き手を直列にしたまま、写しを封じて commit へ
// 渡す。commit が成功したときだけ、メモリ上の vault と baseline を差し替える。
//
// mu は storage が動いているあいだ手放す。storage は世代バックアップを封じるために
// 同じ service を呼ぶからである。mutationMu は持ち続け、別の書き手に追い越させない。
func (s *Service) commitVaultTransaction(transaction vaultTransaction) (storage.Result, error) {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()

	s.mu.Lock()
	vault := s.use()
	if vault == nil {
		s.mu.Unlock()
		exists, err := s.exists()
		if err != nil {
			return storage.Result{}, err
		}
		if exists {
			return storage.Result{}, ErrLocked
		}
		if transaction.commitsWithoutVault {
			return transaction.commit(nil)
		}
		return storage.Result{}, ErrNoVault
	}
	clone := vault.clone()
	published := false
	defer func() {
		if !published {
			clone.Destroy()
		}
	}()
	changed, err := transaction.apply(vault, clone)
	if err != nil {
		s.mu.Unlock()
		return storage.Result{}, err
	}
	if !changed {
		s.mu.Unlock()
		return transaction.commit(nil)
	}
	sealed, err := clone.Seal()
	baseline := slices.Clone(s.baseline)
	s.mu.Unlock()
	if err != nil {
		return storage.Result{}, err
	}
	if len(baseline) == 0 {
		return storage.Result{}, ErrNoVault
	}

	change := storage.Change{
		Path: s.path(), Contents: sealed,
		Precondition: storage.Precondition{Exists: true, Digest: storage.Digest(baseline)},
	}
	result, err := transaction.commit(&change)
	if err != nil {
		return storage.Result{}, err
	}
	s.mu.Lock()
	s.vault.Destroy()
	s.vault = clone
	published = true
	s.baseline = slices.Clone(sealed)
	s.used = s.now()
	s.mu.Unlock()
	return result, nil
}
