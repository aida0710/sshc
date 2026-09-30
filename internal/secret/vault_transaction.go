package secret

import (
	"slices"

	"sshc/internal/storage"
)

// vault と、その鍵に関わる文書だけを書くトランザクションの Operation。
const (
	// operationInitialise は vault を作る。
	operationInitialise = "secret.initialise"
	// operationVault、operationCredentialUpdate は、ロックを解除した vault を書き換える
	// （commitVaultOnlyTransaction）。
	operationVault            = "secret.vault"
	operationCredentialUpdate = "secret.credential.update"
	// operationRekey はマスターパスワードを変える。
	operationRekey = "secret.rekey"
	// operationMigrateKeyBoundArtifacts は、旧バージョンが残した保護文書と控えを
	// 今の形へ移す（migrateKeyBoundArtifacts）。
	operationMigrateKeyBoundArtifacts = "secret.migrate-key-bound-artifacts"
)

// 起動時に AutoUnlock が片付ける保留記録の Operation。どれもほかの設定と一緒に
// 書かないので、利用者の判断を待たずに完了か巻き戻しで片付けてよい。
var (
	// recoveredBeforeUnlock は、vault を開く前に片付ける。どれも巻き戻し用の控えを
	// 封じずに持つ（CommitAtomicDiscardBackups）ので、vault の鍵が要らない。
	// 作成とマスターパスワードの変更は、パスワードなしの vault を開く鍵のファイルも
	// 一緒に書き換えるので、そのファイルを読む前に片付けなければならない。
	recoveredBeforeUnlock = []string{operationInitialise, operationRekey, operationMigrateKeyBoundArtifacts}
	// vaultOnlyOperations は、控えを vault の鍵で封じる（CommitAtomic）。巻き戻しが
	// 控えを開くなら、vault を開いたあとでなければ片付けられない。vault だけを書く
	// 書き手を足したら、ここにも足す。
	vaultOnlyOperations = []string{operationVault, operationCredentialUpdate}
)

// vaultTransaction は、vault の写しへの変更を、呼び手の storage トランザクションと
// 同じ書き込みに載せるための指定である。
//
// vault の書き手（接続の保存、VPN プロファイルの保存、資格情報の追加・更新・削除、
// 鍵のパスフレーズの付け替え）は、どれも「写しを封じて書き、成功したときだけ
// メモリ上の vault を差し替える」手順を踏む。手順そのものはここに 1 か所だけ置き、
// 変更の中身と書き方だけを呼び手が渡す。
type vaultTransaction struct {
	// apply は、写し（clone）へ変更を加え、変わったかを返す。current はロックを
	// 解除した vault そのもので、変わったかを比べるために読むだけである。
	apply func(current, clone *Vault) (bool, error)
	// commitsWithoutVault は、vault がまだ作られていないときに、vault を書かずに
	// commit へ進めてよいかである。消すだけの変更は、消す相手が無いので進めてよい。
	commitsWithoutVault bool
	// missingVaultIsLocked は、vault がまだ作られていないときも ErrLocked を返すかで
	// ある。vault のファイルだけを書く変更は、作られていない vault もロック中と同じに
	// 報告する。
	missingVaultIsLocked bool
	// commit は、呼び手の storage トランザクションを書く。vault を書き換えないときは
	// nil を受け取る。vault を書き換えるときは CommitAtomic で書く。失敗がその場で
	// 巻き戻るので、ディスクの vault だけが先に進む保留記録を作らない。
	commit func(vaultChange *storage.Change) (storage.Result, error)
}

// commitVaultTransaction は、vault の書き手を直列にしたまま、写しを封じて commit へ
// 渡す。commit が成功したときだけ、メモリ上の vault と baseline を差し替える。
//
// mutex は storage が動いているあいだ手放す。storage は世代バックアップを封じるために
// 同じ service を呼ぶからである。mutationMutex は持ち続け、別の書き手に追い越させない。
func (s *Service) commitVaultTransaction(transaction vaultTransaction) (storage.Result, error) {
	s.mutationMutex.Lock()
	defer s.mutationMutex.Unlock()

	s.mutex.Lock()
	vault := s.use()
	if vault == nil {
		s.mutex.Unlock()
		if transaction.missingVaultIsLocked {
			return storage.Result{}, ErrLocked
		}
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
		s.mutex.Unlock()
		return storage.Result{}, err
	}
	if !changed {
		s.mutex.Unlock()
		return transaction.commit(nil)
	}
	sealed, err := clone.Seal()
	baseline := slices.Clone(s.baseline)
	s.mutex.Unlock()
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
	s.mutex.Lock()
	s.vault.Destroy()
	s.vault = clone
	published = true
	s.baseline = slices.Clone(sealed)
	s.used = s.now()
	s.mutex.Unlock()
	return result, nil
}

// commitVaultOnlyTransaction は、vault のファイルだけを書く変更を
// commitVaultTransaction に載せる。apply は写しへ変更を加え、変わったかを返す。
//
// CommitAtomic で書くので、rename や SyncDir が失敗したときはその場で巻き戻る。
// 履歴の画面から「完了させる」ことのできる保留記録を残さず、ディスクだけが新しい
// vault になってメモリ上の vault が古いまま残ることがない。
func (s *Service) commitVaultOnlyTransaction(operation string, apply func(clone *Vault) (bool, error)) error {
	_, err := s.commitVaultTransaction(vaultTransaction{
		apply: func(_, clone *Vault) (bool, error) {
			return apply(clone)
		},
		missingVaultIsLocked: true,
		commit: func(change *storage.Change) (storage.Result, error) {
			if change == nil {
				return storage.Result{}, nil
			}
			return s.transactions.CommitAtomic(storage.Request{
				Operation: operation,
				Changes:   []storage.Change{*change},
			})
		},
	})
	return err
}
