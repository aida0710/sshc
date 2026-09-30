package application

import (
	"errors"

	"sshc/internal/secret"
	"sshc/internal/storage"
)

// StartupRenamer prepares an encrypted binding change under the vault's stable
// generation. The callback commits it together with the SSH configuration.
type StartupRenamer interface {
	WithStartupRename(from, to string, commit func(*storage.Change) (storage.Result, error)) (storage.Result, error)
}

// ErrStartupRenamerMissing は、Vault を開いたままの改名で、起動時コマンドを移す
// StartupRenamer が渡されていないことを表す。nil を「起動時コマンドが無い」と
// 読むと、配線を忘れた構成が起動時コマンドを旧 alias に残したまま成功を返す。
var ErrStartupRenamerMissing = errors.New("alias rename has no startup command renamer")

// NoStartupRenamer は、起動時コマンドを持たない構成（テストなど）が明示で渡す
// StartupRenamer である。設定と Vault だけを書く。
type NoStartupRenamer struct{}

func (NoStartupRenamer) WithStartupRename(_, _ string, commit func(*storage.Change) (storage.Result, error)) (storage.Result, error) {
	return commit(nil)
}

// SetStartupRenamer は、Vault を開いたままの改名に必須である。起動時コマンドを
// 持たない構成は NoStartupRenamer を渡す。
func (s *Service) SetStartupRenamer(renamer StartupRenamer) {
	s.startupRenamer = renamer
}

// SaveWithSecrets owns the whole alias transition. A rename moves the vault
// assignments and the startup snippet to the new alias; a raw edit that removes
// a Host declaration drops that alias's startup snippet. A locked vault retains
// the existing contract: config and public metadata can still be saved alone.
func (s *Service) SaveWithSecrets(request EditRequest) (SaveResult, error) {
	vault := s.vault
	if vault == nil || !vault.Unlocked() {
		return s.Save(request)
	}
	switch request.Kind {
	case EditRename:
		return s.saveAliasRename(vault, request)
	case EditFileRaw, EditBlockRaw:
		return s.saveAliasRemoval(vault, request)
	default:
		return s.Save(request)
	}
}

func (s *Service) saveAliasRename(vault *secret.Service, request EditRequest) (SaveResult, error) {
	renamer := s.startupRenamer
	if renamer == nil {
		return SaveResult{}, ErrStartupRenamerMissing
	}
	var saved SaveResult
	_, err := vault.WithConnectionSecretsTransaction(secret.ConnectionSecretsMutation{
		Rename: &secret.AliasRename{From: request.Alias, To: request.NewAlias},
	}, func(vaultChange *storage.Change) (storage.Result, error) {
		commit := func(startupChange *storage.Change) (storage.Result, error) {
			updated, committed, err := s.commitWithSealedChanges(request, []*storage.Change{vaultChange, startupChange})
			saved = updated
			return committed, err
		}
		return renamer.WithStartupRename(request.Alias, request.NewAlias, commit)
	})
	return saved, err
}

// commitWithSealedChanges plans request and commits it atomically together
// with the already sealed vault and startup documents; nil changes are skipped.
func (s *Service) commitWithSealedChanges(request EditRequest, changes []*storage.Change) (SaveResult, storage.Result, error) {
	s.saveMutex.Lock()
	defer s.saveMutex.Unlock()
	prepared, err := s.plan(request)
	if err != nil {
		return SaveResult{}, storage.Result{}, err
	}
	storageRequest := s.requestFor(prepared)
	storageRequest.Directories = append(storageRequest.Directories, storage.DirectoryCreate{Path: s.workspace.StateDir()})
	for _, change := range changes {
		if change != nil {
			storageRequest.Changes = append(storageRequest.Changes, *change)
		}
	}
	committed, err := s.commitAtomicPlannedRequest(prepared, storageRequest)
	if err != nil {
		return SaveResult{}, committed, err
	}
	return s.connectionUpdateResult(committed, prepared), committed, nil
}
