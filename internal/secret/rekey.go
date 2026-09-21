package secret

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"sshc/internal/envelope"
	"sshc/internal/storage"
)

// SealBackup は世代バックアップをひとつ暗号化、OpenBackup はそれを開く。
//
// これらはストレージ層に import されるのではなく、そこへ渡される。秘密がどこに
// あるかはこのパッケージの領分であり、トランザクションマネージャがそれを知らねば
// ならない道理はない。閉じた vault は何も暗号化せず、何も開かない、アプリケーションは
// マスターパスワードの後ろにあるので、何かが書かれている最中にそれが起きることは
// ないし、万一起きたなら、ここで失敗するのが正しい結果である。もう一方の選択肢は、
// 秘密鍵の平文コピーだからだ。
func (s *Service) SealBackup(plaintext []byte) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.backupVault != nil {
		return s.backupVault.SealBytes(plaintext)
	}
	vault := s.use()
	if vault == nil {
		return nil, ErrLocked
	}
	return vault.SealBytes(plaintext)
}

func (s *Service) OpenBackup(sealed []byte) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault := s.use()
	if vault == nil {
		return nil, ErrLocked
	}
	return vault.OpenBytes(sealed)
}

// ChangeMasterPassword は、鍵を導出し直し、その鍵が保持していたすべてを暗号化し直す。
//
// vault も、暗号化されたオブジェクトストアの設定も、すべての世代バックアップも、
// マスターパスワードから導出された鍵で暗号化されている。vault だけを置き換える変更は、
// 残りを、もう誰も使わないパスワードで開ける状態のまま残す。それは失うのと同じ
// ことだ。バックアップは、そこから復元するために存在するのであり、誰にも開けない
// バックアップはバックアップではない。
//
// トランザクションはひとつ。古い暗号文は適用途中の巻き戻しにだけ使う一時的な控えへ
// 保存し、commit point を越えたら破棄する。commit point より前のクラッシュは全対象を
// 古い世代へ、以後のクラッシュは全対象を新しい世代へ収束させる。
//
// リモートのスナップショットは独立した同期鍵で暗号化されているため、マスターパスワード
// の変更とは無関係である。この関数はローカルの vault、同期設定、世代バックアップだけを
// 再暗号化する。
func (s *Service) ChangeMasterPassword(current, next string) error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	if ok, err := s.Verify(current); err != nil {
		return err
	} else if !ok {
		return ErrWrongPassphrase
	}

	current, err := s.resolvePassphrase(current)
	if err != nil {
		return err
	}
	localChange, effective, err := s.prepareProtection(next)
	if err != nil {
		return err
	}
	defer clear(localChange.Contents)

	s.mu.Lock()
	vault := s.use()
	if vault == nil {
		s.mu.Unlock()
		return ErrLocked
	}
	candidate := vault.clone()
	previous, err := candidate.Rekey(effective)
	if err != nil {
		candidate.Destroy()
		s.mu.Unlock()
		return err
	}
	// 稼働中の vault は commit point まで古い鍵のまま保つ。候補だけが新しい鍵で
	// 暗号化し、失敗経路でプロセス内状態を戻す必要そのものをなくす。
	changes, buildErr := s.reSealKeyBoundArtifacts(candidate, previous, current, false, true)
	if buildErr != nil {
		previous.Destroy()
		candidate.Destroy()
		s.mu.Unlock()
		return buildErr
	}
	sealed, sealErr := candidate.Seal()
	previous.Destroy()
	s.mu.Unlock()
	if sealErr != nil {
		candidate.Destroy()
		return sealErr
	}

	previousVault, readErr := s.workspace.FileSystem().ReadFile(s.path())
	if readErr != nil {
		candidate.Destroy()
		return readErr
	}
	changes = append(changes, storage.Change{
		Path: s.path(), Contents: sealed,
		Precondition: storage.Precondition{Exists: true, Digest: storage.Digest(previousVault)},
	})
	if _, err := s.transactions.CommitAtomicDiscardBackupsAndPublish(storage.Request{
		Operation: "secret.rekey",
		Changes:   append(changes, localChange),
	}, func() {
		// Publish while the workspace mutation barrier still excludes a normal
		// commit. Its SealBackup callback can therefore observe only the new key
		// after the rekeyed backup set is durable.
		s.mu.Lock()
		if s.vault != nil && s.vault != candidate {
			s.vault.Destroy()
		}
		s.passwordless = next == ""
		s.vault = candidate
		s.baseline = slices.Clone(sealed)
		s.mu.Unlock()
	}); err != nil {
		candidate.Destroy()
		return err
	}
	return nil
}

// reSealed は、古い鍵が暗号化したすべてのファイルを読み、新しい鍵で暗号化し直す。
//
// バックアップはマネージャ経由ではなく直接読む。マネージャは現在の新しい鍵を
// 使用するため、古い鍵で暗号化されたバックアップを開けない。
func (s *Service) reSealKeyBoundArtifacts(vault *Vault, previous envelope.Key, passphrase string, skipBackup, includeSettings bool) ([]storage.Change, error) {
	changes, err := s.reSealProtectedDocuments(vault, previous, passphrase, skipBackup)
	if err != nil {
		return nil, fmt.Errorf("re-seal protected documents: %w", err)
	}

	settings, err := s.workspace.FileSystem().ReadFile(s.settingsPath())
	switch {
	case err == nil && includeSettings:
		plaintext, openErr := openWithKnownGeneration(settings, previous, passphrase)
		if openErr != nil {
			return nil, fmt.Errorf("open synchronization settings: %w", openErr)
		}
		defer clear(plaintext)
		resealed, sealErr := vault.SealBytes(plaintext)
		if sealErr != nil {
			return nil, sealErr
		}
		changes = append(changes, storage.Change{
			Path: s.settingsPath(), Contents: resealed,
			Precondition: storage.Precondition{Exists: true, Digest: storage.Digest(settings)},
			SkipBackup:   skipBackup,
		})
	case err == nil, errors.Is(err, fs.ErrNotExist):
	default:
		return nil, err
	}

	backups := filepath.Join(s.workspace.StateDir(), storage.BackupDirectoryName)
	walkErr := filepath.WalkDir(backups, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if entry.IsDir() {
			return nil
		}
		body, readErr := s.workspace.ReadTransactionFile(path)
		if readErr != nil {
			return readErr
		}
		resealed, sealErr := s.reSealBackup(path, body, vault, previous, passphrase)
		if sealErr != nil {
			relative, relativeErr := filepath.Rel(backups, path)
			if relativeErr != nil {
				relative = filepath.Base(path)
			}
			return fmt.Errorf("re-seal backup %s: %w", filepath.ToSlash(relative), sealErr)
		}
		changes = append(changes, storage.Change{
			Path: path, Contents: resealed,
			Precondition: storage.Precondition{Exists: true, Digest: storage.Digest(body)},
			SkipBackup:   skipBackup,
		})
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, fs.ErrNotExist) {
		return nil, walkErr
	}
	return changes, nil
}

func openWithKnownGeneration(sealed []byte, previous envelope.Key, passphrase string) ([]byte, error) {
	plaintext, err := previous.Open(sealed)
	if err == nil || passphrase == "" {
		return plaintext, err
	}
	plaintext, key, fallbackErr := envelope.Open(sealed, passphrase)
	key.Destroy()
	if fallbackErr != nil {
		return nil, err
	}
	return plaintext, nil
}

func (s *Service) reSealBackup(path string, body []byte, vault *Vault, previous envelope.Key, passphrase string) ([]byte, error) {
	plaintext, err := openWithKnownGeneration(body, previous, passphrase)
	if err != nil {
		// A single unreadable backup blocks a key-generation change. Ignoring it
		// would make that generation permanently inaccessible after publication.
		return nil, fmt.Errorf("open outer envelope: %w", err)
	}
	defer clear(plaintext)
	keyBound, validate := s.backupKeyBoundDocument(path)
	if keyBound {
		inner, openErr := openWithKnownGeneration(plaintext, previous, passphrase)
		if errors.Is(openErr, envelope.ErrNotAnEnvelope) {
			// Older test fixtures and pre-backup-encryption generations may contain
			// the key-bound document itself rather than outer-envelope(inner-envelope).
			// The first successful open authenticated those bytes; normalize them to
			// the current nested backup form before publishing the new generation.
			inner = slices.Clone(plaintext)
			openErr = nil
		}
		if openErr != nil {
			return nil, fmt.Errorf("open inner envelope: %w", openErr)
		}
		defer clear(inner)
		if validate != nil {
			if validateErr := validate(inner); validateErr != nil {
				return nil, validateErr
			}
		}
		plaintext, err = vault.SealBytes(inner)
		if err != nil {
			return nil, err
		}
		defer clear(plaintext)
	}
	return vault.SealBytes(plaintext)
}

func (s *Service) backupKeyBoundDocument(path string) (bool, func([]byte) error) {
	root := filepath.Join(s.workspace.StateDir(), storage.BackupDirectoryName)
	relative, err := filepath.Rel(root, path)
	if err != nil || filepath.IsAbs(relative) {
		return false, nil
	}
	parts := strings.Split(filepath.Clean(relative), string(filepath.Separator))
	if len(parts) < 2 {
		return false, nil
	}
	original := filepath.Join(s.workspace.Root(), filepath.Join(parts[1:]...))
	if original == s.path() || original == s.settingsPath() {
		return true, nil
	}
	for _, document := range s.protectedDocuments {
		if original == document.Path {
			return true, document.Validate
		}
	}
	return false, nil
}

func (s *Service) reSealProtectedDocuments(vault *Vault, previous envelope.Key, passphrase string, skipBackup bool) ([]storage.Change, error) {
	changes := make([]storage.Change, 0, len(s.protectedDocuments))
	for _, document := range s.protectedDocuments {
		body, err := s.workspace.FileSystem().ReadFile(document.Path)
		switch {
		case err == nil:
			plaintext, openErr := openWithKnownGeneration(body, previous, passphrase)
			if errors.Is(openErr, envelope.ErrNotAnEnvelope) {
				// A valid legacy plaintext document may not have been opened since
				// the encryption feature was installed. Rotate it directly into the
				// candidate generation rather than publishing a new key first.
				plaintext = body
			} else if openErr != nil {
				return nil, openErr
			}
			if validateErr := document.Validate(plaintext); validateErr != nil {
				return nil, validateErr
			}
			resealed, sealErr := vault.SealBytes(plaintext)
			if sealErr != nil {
				return nil, sealErr
			}
			changes = append(changes, storage.Change{
				Path: document.Path, Contents: resealed,
				Precondition: storage.Precondition{Exists: true, Digest: storage.Digest(body)},
				SkipBackup:   skipBackup,
			})
		case errors.Is(err, fs.ErrNotExist):
		default:
			return nil, err
		}
	}
	return changes, nil
}

// TravelDocument は、同期用に復号した vault 文書を返す。
// ディスク上の vault は端末固有のマスターパスワードで暗号化されているため、
// その暗号文自体は同期しない。
func (s *Service) TravelDocument() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault := s.use()
	if vault == nil {
		return nil, ErrLocked
	}
	// 空の保管庫は運ばない。運ぶものが無いだけでなく、運べば 2 台目の最初の
	// pull が必ず衝突する。空であることは編集ではない。
	if vault.Empty() {
		return nil, nil
	}
	return vault.Document()
}

// EmptyTravelDocument returns the canonical logical document for an empty
// vault while proving that this installation currently owns an unlocked master
// key generation. Remote synchronization uses it to represent an intentional
// remote deletion; AdoptTravelDocument then seals the same document with the
// receiving installation's current key.
func (s *Service) EmptyTravelDocument() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault := s.use()
	if vault == nil {
		return nil, ErrLocked
	}
	secrets, subjects := newMaps()
	empty := &Vault{
		key: vault.key.Clone(), secrets: secrets, subjects: subjects,
		dedicatedPasswords: map[string]string{}, passwordBindings: map[string]string{},
		dedicatedKeyPassphrases: map[string]string{},
	}
	defer empty.Destroy()
	return empty.Document()
}

// AdoptTravelDocument は、受信した vault 文書をこの端末の鍵で暗号化する。
// 書き込みは呼び出し側がトランザクションとして行う。
func (s *Service) AdoptTravelDocument(plain []byte) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(plain) > storage.MaxFileSize {
		return nil, storage.ErrFileTooLarge
	}
	vault := s.use()
	if vault == nil {
		return nil, ErrLocked
	}
	// 現行文書として完全に開けないものを暗号化しない。旧schemaや未知fieldを
	// そのまま保存する互換経路は持たず、現行の正規形だけを採用する。
	incoming, err := openDocument(plain, vault.key.Clone())
	if err != nil {
		return nil, err
	}
	defer incoming.Destroy()
	canonical, err := incoming.Document()
	if err != nil {
		return nil, err
	}
	if len(canonical) > storage.MaxFileSize {
		return nil, storage.ErrFileTooLarge
	}
	sealed, err := vault.SealBytes(canonical)
	if err != nil {
		return nil, err
	}
	if len(sealed) > storage.MaxFileSize {
		return nil, storage.ErrFileTooLarge
	}
	return sealed, nil
}
