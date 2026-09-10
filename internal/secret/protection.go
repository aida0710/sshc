package secret

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io/fs"
	"path/filepath"

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
func (s *Service) AutoUnlock() error {
	// Restore one complete generation before consulting the device key. These
	// atomic transactions keep raw rollback material and need no unlocked vault.
	pending, err := s.transactions.Pending()
	if err != nil {
		return err
	}
	for _, item := range pending {
		if item.Operation != "secret.vault" && item.Operation != "secret.rekey" {
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

	body, err := s.localKey()
	if err != nil {
		return err
	}
	defer clear(body)
	if len(body) == 0 {
		return nil
	}
	return s.Unlock("")
}
