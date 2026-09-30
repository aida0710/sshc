package secret

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"

	"sshc/internal/storage"
)

// LocalKeyPath never travels to another device. An empty file means password
// protection; otherwise it holds a random 256-bit device unlock secret. Keeping
// this marker as a file lets the existing atomic rekey transaction switch both
// the vault and its protection mode together without retaining old backups.
const LocalKeyPath = "sshc/local-vault-key"

func (s *Service) localKey() ([]byte, error) {
	path, err := s.workspace.ResolveForWrite(filepath.Join(s.workspace.Root(), filepath.FromSlash(LocalKeyPath)))
	if errors.Is(err, storage.ErrMissingDirectory) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	body, err := s.workspace.FileSystem().ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(body) != 0 && len(body) != 32 {
		return nil, ErrNotAVault
	}
	return body, nil
}

// unlockPasswordlessHeld は、施錠中のパスワードなしの Vault を解錠する。呼び手が
// mutationMutex を持っている。パスワードのある Vault と解錠中の Vault には何もしない。
//
// 呼び手が錠を持っているので afterUnlock へは知らせない。鍵を付ける前の同期状態の
// digest は、次の Unlock か engine の起動のときに移す。
func (s *Service) unlockPasswordlessHeld() error {
	if s.Unlocked() {
		return nil
	}
	passwordless, err := s.hasLocalKey()
	if err != nil || !passwordless {
		return err
	}
	_, err = s.autoUnlockHeld()
	return err
}

// hasLocalKey は、このマシンに解錠用の鍵がある（パスワードなしの Vault）かを返す。
func (s *Service) hasLocalKey() (bool, error) {
	body, err := s.localKey()
	if err != nil {
		return false, err
	}
	defer clear(body)
	return len(body) > 0, nil
}

func (s *Service) resolvePassphrase(passphrase string) (string, error) {
	if passphrase != "" {
		return passphrase, nil
	}
	body, err := s.localKey()
	if err != nil {
		return "", err
	}
	defer clear(body)
	if len(body) == 0 {
		return "", ErrWrongPassphrase
	}
	return hex.EncodeToString(body), nil
}

func (s *Service) prepareProtection(passphrase string) (storage.Change, string, error) {
	old, err := s.localKey()
	if err != nil {
		return storage.Change{}, "", err
	}
	defer clear(old)
	path := filepath.Join(s.workspace.Root(), filepath.FromSlash(LocalKeyPath))
	precondition := storage.Precondition{}
	if _, err := s.workspace.FileSystem().Lstat(path); err == nil {
		precondition = storage.Precondition{Exists: true, Digest: storage.Digest(old)}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return storage.Change{}, "", err
	}
	var body []byte
	if passphrase == "" {
		body = make([]byte, 32)
		if _, err := rand.Read(body); err != nil {
			return storage.Change{}, "", err
		}
		passphrase = hex.EncodeToString(body)
	}
	return storage.Change{Path: path, Contents: body, Precondition: precondition}, passphrase, nil
}

// AutoUnlock is called once by the engine after all protected documents have
// been registered. Explicit Lock still destroys the in-memory key.
//
// 中断した vault の変更の保留記録も、ここで片付ける。vault の鍵が要らない記録は
// 鍵を読む前に、要る記録は自動で解錠できたあとに片付ける。マスターパスワードの
// vault で鍵が要る記録は残し、利用者が解錠してから履歴の画面で片付ける。起動時に
// 鍵の無いまま巻き戻そうとすると ErrLocked で engine が起動できない。
func (s *Service) AutoUnlock() error {
	s.mutationMutex.Lock()
	unlocked, err := s.autoUnlockHeld()
	s.mutationMutex.Unlock()
	if unlocked {
		s.notifyUnlocked()
	}
	return err
}

// autoUnlockHeld は AutoUnlock の本体である。呼び手が mutationMutex を持っている。
// パスワードなしの Vault を解錠できたかを返す。afterUnlock へは知らせないので、
// 呼び手が mutationMutex を放してから notifyUnlocked で知らせる。
//
// 中断した secret.vault と secret.rekey を完了・巻き戻しするので、mutationMutex の外で
// 走らせてはならない。storage の Pending は実行中のトランザクションの記録も読むので、
// 走っている ChangeMasterPassword の rekey を、中断したものとして扱ってしまう。
func (s *Service) autoUnlockHeld() (bool, error) {
	if err := s.recoverPending(func(item storage.Pending) bool {
		return slices.Contains(recoveredBeforeUnlock, item.Operation) ||
			slices.Contains(vaultOnlyOperations, item.Operation) && !rollbackOpensSealedBackup(item)
	}); err != nil {
		return false, err
	}

	passwordless, err := s.hasLocalKey()
	if err != nil || !passwordless {
		return false, err
	}
	if err := s.unlockHeld(""); err != nil {
		return false, err
	}
	return true, s.recoverPending(func(item storage.Pending) bool {
		return slices.Contains(vaultOnlyOperations, item.Operation)
	})
}

// recoverPending は、recoverable が選んだ保留記録を、完了か巻き戻しで片付ける。
func (s *Service) recoverPending(recoverable func(storage.Pending) bool) error {
	pending, err := s.transactions.Pending()
	if err != nil {
		return err
	}
	for _, item := range pending {
		if !recoverable(item) {
			continue
		}
		switch {
		case item.CanComplete:
			err = s.transactions.Complete(item.ID)
		case item.CanRollback:
			err = s.transactions.Rollback(item.ID)
		default:
			return storage.ErrRecoveryStateUnknown
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// rollbackOpensSealedBackup は、vault だけを書く変更の保留記録を巻き戻すときに、
// vault の鍵で封じた控えを開く必要があるかを返す。巻き戻しが控えを読むのは、適用
// 済みで控えを持つエントリだけである。
func rollbackOpensSealedBackup(item storage.Pending) bool {
	for _, entry := range item.Entries {
		if entry.Committed && entry.HasBackup {
			return true
		}
	}
	return false
}
