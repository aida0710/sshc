package secret

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"sshc/internal/envelope"
	"sshc/internal/storage"
)

// 旧バージョンが残した、vault の鍵に結び付いたファイルを今の形へ移す。
//
// 今の形は 2 つの約束でできている。保護文書（sshc/snippets.json）は vault の鍵で
// 封じてある。保護文書の控えは入れ子で、外側の封の中にも vault の鍵の封がある。
// sshc/snippets.json が平文だった v0.6.0〜v0.20.x は、この約束を守らないファイルを
// 2 種類残した。平文の保護文書と、pull が平文の保護文書を置き換えたときの、外側の封の
// 中に平文の JSON がある（入れ子になっていない）控えである。
//
// 古い形を読むのはここだけである。ロックを解除したときに移し、鍵を変える変更
// （マスターパスワードの変更、未対応の vault の復旧とリセット）は、封じ直す前に移し
// 終える。封じ直す処理（reSealKeyBoundArtifacts）は今の形しか読まない。

// keyBoundMigration は、古い形のファイルを今の形へ移すときに使う鍵。
type keyBoundMigration struct {
	// Key は、今の Vault の鍵。移したファイルはすべてこの鍵で封じる。
	Key envelope.Key
	// Passphrase は、Key で開けない控えの外側を開くマスターパスワード（パスワードなしの
	// Vault では、resolvePassphrase が local-vault-key から作る値）。reSealBackup と同じく、
	// 前の世代の鍵で封じた控えを開くのに使う。
	Passphrase string
}

// migrateKeyBoundArtifacts は、Vault の鍵で封じるべきファイルのうち古い形のものを、
// migration.Key で今の形へ移す。移すものが無ければ何も書かない。
//
// 控えの外側は、migration.Key で開けなければ migration.Passphrase で開く。v0.16〜v0.20 の
// 未対応の Vault の復旧とリセットは Vault だけを置き換え、控えを封じ直さなかった。その
// ため、同じマスターパスワードから作った前の世代の鍵で外側を封じた控えが残りうる。
// どちらでも開けない控えは移さず、鍵を変える変更の封じ直しが理由を示して断る。
//
// 呼び出し側は mutationMutex を持つ。保護文書を書くほかの経路（スニペットの保存と
// 同期の受信）も同じロックを取るので、読んでから書くまでに中身が入れ替わらない。
func (s *Service) migrateKeyBoundArtifacts(migration keyBoundMigration) error {
	documents, err := s.plaintextDocumentChanges(migration.Key)
	if err != nil {
		return err
	}
	backups, err := s.unnestedBackupChanges(migration)
	if err != nil {
		return err
	}
	changes := append(documents, backups...)
	if len(changes) == 0 {
		return nil
	}
	// 移す前の内容は、中断したときの巻き戻しにだけ使い、終わったら捨てる。封じて
	// 控えに残すと、平文の保護文書から入れ子でない控えをもう一度作ってしまう。
	_, err = s.transactions.CommitAtomicDiscardBackups(storage.Request{
		Operation: operationMigrateKeyBoundArtifacts, Changes: changes,
	})
	return err
}

// plaintextDocumentChanges は、平文のまま残った保護文書を key で封じる変更を返す。
func (s *Service) plaintextDocumentChanges(key envelope.Key) ([]storage.Change, error) {
	var changes []storage.Change
	for _, document := range s.protectedDocuments {
		body, err := s.workspace.FileSystem().ReadFile(document.Path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if envelope.IsEnvelope(body) {
			continue
		}
		if err := document.Validate(body); err != nil {
			return nil, fmt.Errorf("validate the plaintext %s: %w", filepath.Base(document.Path), err)
		}
		sealed, err := key.Seal(body)
		if err != nil {
			return nil, err
		}
		changes = append(changes, storage.Change{
			Path: document.Path, Contents: sealed,
			Precondition: storage.Precondition{Exists: true, Digest: storage.Digest(body)},
		})
	}
	return changes, nil
}

// unnestedBackupChanges は、保護文書の控えのうち入れ子になっていないものを、入れ子に
// して migration.Key で封じ直す変更を返す。
func (s *Service) unnestedBackupChanges(migration keyBoundMigration) ([]storage.Change, error) {
	backups := filepath.Join(s.workspace.StateDir(), storage.BackupDirectoryName)
	records, err := s.workspace.FileSystem().ReadDir(backups)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var changes []storage.Change
	for _, record := range records {
		if !record.IsDir() {
			continue
		}
		for _, document := range s.protectedDocuments {
			relative, err := filepath.Rel(s.workspace.Root(), document.Path)
			if err != nil {
				return nil, err
			}
			change, unnested, err := s.nestedBackupChange(filepath.Join(backups, record.Name(), relative), document, migration)
			if err != nil {
				return nil, fmt.Errorf("nest the backup %s: %w", filepath.ToSlash(filepath.Join(record.Name(), relative)), err)
			}
			if unnested {
				changes = append(changes, change)
			}
		}
	}
	return changes, nil
}

// nestedBackupChange は、path の控えが入れ子になっていなければ、入れ子にして
// migration.Key で封じ直す変更を返す。控えが無い、外側を開けない、すでに入れ子である、
// のどれかなら何も返さない。
func (s *Service) nestedBackupChange(path string, document ProtectedDocument, migration keyBoundMigration) (storage.Change, bool, error) {
	body, err := s.workspace.ReadTransactionFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return storage.Change{}, false, nil
	}
	if err != nil {
		return storage.Change{}, false, err
	}
	outer, err := openWithKnownGeneration(body, migration.Key, migration.Passphrase)
	if err != nil {
		return storage.Change{}, false, nil
	}
	defer clear(outer)
	if envelope.IsEnvelope(outer) {
		return storage.Change{}, false, nil
	}
	// 外側を開けたので、中身は認証済みである。
	if err := document.Validate(outer); err != nil {
		return storage.Change{}, false, err
	}
	inner, err := migration.Key.Seal(outer)
	if err != nil {
		return storage.Change{}, false, err
	}
	nested, err := migration.Key.Seal(inner)
	if err != nil {
		return storage.Change{}, false, err
	}
	return storage.Change{
		Path: path, Contents: nested,
		Precondition: storage.Precondition{Exists: true, Digest: storage.Digest(body)},
	}, true, nil
}
