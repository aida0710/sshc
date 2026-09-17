package secret

import (
	"errors"
	"io/fs"
	"path/filepath"

	"sshc/internal/storage"
)

// settingsPath は、vault の隣にある、暗号化されたオブジェクトストアの設定。
func (s *Service) settingsPath() string {
	return filepath.Join(s.workspace.Root(), filepath.FromSlash(SettingsPath))
}

// SyncSettings は、秘密も含めてオブジェクトストアの設定を返す。
//
// これを求めるのはクライアントを組み立てる呼び出し側だけである。画面には、秘密で
// ないフィールドから返す。これらを一度も与えられていないマシンはゼロ値を返し、
// エラーにはしない。「まだ設定されていない」は状態であって、失敗では
// ないからだ。
func (s *Service) SyncSettings() (SyncSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault := s.use()
	if vault == nil {
		return SyncSettings{}, ErrLocked
	}
	sealed, err := s.workspace.FileSystem().ReadFile(s.settingsPath())
	if errors.Is(err, fs.ErrNotExist) {
		return SyncSettings{}, nil
	}
	if err != nil {
		return SyncSettings{}, err
	}
	return vault.OpenSettings(sealed)
}

// SetSyncSettings は、オブジェクトストアの設定を置き換える。
//
// 同期の鍵だけは、空で渡されても消さない。設定の form はその欄を持たない
// 鍵を見せるのは作った一度だけで、以後は伏せ字である。ので、素直に置き換えれば、
// bucket を編集しただけでリモートのスナップショットが誰にも開けなくなる。
func (s *Service) SetSyncSettings(settings SyncSettings) error {
	return s.writeSyncSettings(func(stored SyncSettings) (SyncSettings, error) {
		if settings.Key == "" {
			settings.Key = stored.Key
		}
		// 自動同期の入切も form の欄ではない。bucket を編集しただけで、
		// 巡回が暗黙に止まってはならない。
		settings.Auto = stored.Auto
		return settings, nil
	})
}

// SetSyncAuto は、自動同期の入切だけを置き換える。
func (s *Service) SetSyncAuto(enabled bool) error {
	return s.writeSyncSettings(func(stored SyncSettings) (SyncSettings, error) {
		stored.Auto = enabled
		return stored, nil
	})
}

// SetSyncKey は、同期の鍵だけを置き換える。他の設定はそのまま残る。
func (s *Service) SetSyncKey(key string) error {
	return s.writeSyncSettings(func(stored SyncSettings) (SyncSettings, error) {
		stored.Key = key
		return stored, nil
	})
}

var ErrSyncSettingsChanged = errors.New("the synchronization settings changed")

// SetSyncKeyIfSettingsMatch commits a rotated key only to the exact remote
// binding whose ciphertext was changed. Auto is deliberately excluded: toggling
// the scheduler does not change a remote target generation.
func (s *Service) SetSyncKeyIfSettingsMatch(expected SyncSettings, key string) error {
	return s.writeSyncSettings(func(stored SyncSettings) (SyncSettings, error) {
		if stored.Endpoint != expected.Endpoint || stored.Bucket != expected.Bucket ||
			stored.Path != expected.Path || stored.Region != expected.Region ||
			stored.AccessKeyID != expected.AccessKeyID || stored.SecretAccessKey != expected.SecretAccessKey ||
			stored.Direction != expected.Direction || stored.Key != expected.Key {
			return SyncSettings{}, ErrSyncSettingsChanged
		}
		stored.Key = key
		return stored, nil
	})
}

// writeSyncSettings は、いま保存されているものを読み、mutate に渡し、返ってきた
// ものを暗号化して書く。読みと書きは同じ mutationMu の下で起きるので、二つの呼び出しが
// 互いの結果を踏まない。
func (s *Service) writeSyncSettings(mutate func(SyncSettings) (SyncSettings, error)) error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	s.mu.Lock()
	vault := s.use()
	if vault == nil {
		s.mu.Unlock()
		return ErrLocked
	}
	stored := SyncSettings{}
	existing, readErr := s.workspace.FileSystem().ReadFile(s.settingsPath())
	switch {
	case readErr == nil:
		opened, err := vault.OpenSettings(existing)
		if err != nil {
			s.mu.Unlock()
			return err
		}
		stored = opened
	case !errors.Is(readErr, fs.ErrNotExist):
		s.mu.Unlock()
		return readErr
	}
	next, err := mutate(stored)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	sealed, err := vault.SealSettings(next)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if err := s.workspace.EnsureDirectory(s.workspace.StateDir()); err != nil {
		return err
	}
	precondition := storage.Precondition{}
	if readErr == nil {
		// Bind the write to the exact document which mutate consumed. Re-reading
		// here and accepting that digest would silently overwrite an external
		// writer which won the race between decrypt and commit.
		precondition = storage.Precondition{Exists: true, Digest: storage.Digest(existing)}
	} else if !errors.Is(readErr, fs.ErrNotExist) {
		return readErr
	}
	_, err = s.transactions.Commit(storage.Request{
		Operation: "sync.settings",
		Changes: []storage.Change{{
			Path: s.settingsPath(), Contents: sealed, Precondition: precondition,
		}},
	})
	return err
}
