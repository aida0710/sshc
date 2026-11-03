package application

import (
	"errors"

	"sshc/internal/secret"
	"sshc/internal/sshclient"
	"sshc/internal/storage"
	"sshc/internal/terminal"
)

// OffersPasswordSave は、target へ打ったパスワードで接続できたときに、Vault に保存するかを
// 尋ねてよいかを返す。保存しても使われないパスワード（パスワード認証を切っている、IdentityFile を
// 直接書いている）と、保存先の接続名が無い接続先は尋ねない。一度断った認証先も尋ねない。
func (s *Service) OffersPasswordSave(target sshclient.Target) bool {
	if target.Direct || s.vault == nil || !s.vault.Unlocked() {
		return false
	}
	eligibility, err := s.PasswordEligibility(target.Alias)
	if err != nil || !eligibility.Storable {
		return false
	}
	graph, err := s.resolve()
	if err != nil {
		return false
	}
	identity := s.connectionIdentity(graph, target.Alias)
	if identity.IsZero() {
		return false
	}
	stored, _, err := s.metadata.Load()
	if err != nil {
		return false
	}
	index := hostMetadataIndex(stored.Hosts, identity)
	return index < 0 || stored.Hosts[index].PasswordSaveDeclinedBinding != target.AuthenticationBinding()
}

// SaveTypedPassword は、ターミナルで打って認証が通ったパスワードを、接続名の専用のパスワードとして
// Vault に保存する。接続の保存と同じ変更（PasswordMutationDedicated）である。打ったあとで接続先の
// 設定が変わっていたら、別の相手のパスワードにならないように ErrConnectionChanged で断る。
func (s *Service) SaveTypedPassword(typed terminal.TypedPassword) error {
	if s.vault == nil {
		return secret.ErrNoVault
	}
	_, err := s.vault.WithPasswordMutation(secret.PasswordMutation{
		Kind: secret.PasswordMutationDedicated, Alias: typed.Alias, Password: typed.Password, Binding: typed.Binding,
	}, func(change storage.Change) (storage.Result, error) {
		s.saveMutex.Lock()
		defer s.saveMutex.Unlock()
		if err := s.refuseChangedBinding(typed); err != nil {
			return storage.Result{}, err
		}
		return s.manager.CommitAtomic(storage.Request{Operation: "secret.save-typed-password", Changes: []storage.Change{change}})
	})
	if errors.Is(err, secret.ErrNoPasswordMutation) {
		return nil
	}
	return err
}

func (s *Service) refuseChangedBinding(typed terminal.TypedPassword) error {
	graph, err := s.resolve()
	if err != nil {
		return err
	}
	stored, _, err := s.metadata.Load()
	if err != nil {
		return err
	}
	binding, err := s.passwordBindingForGraph(graph, stored, typed.Alias)
	if err != nil {
		return err
	}
	if binding != typed.Binding {
		return ErrConnectionChanged
	}
	return nil
}

// DeclineTypedPassword は、ターミナルで打ったパスワードを保存しないと答えたことを覚える。同じ
// 認証先へまた打って接続しても、もう尋ねない。接続先を変えたら別の相手なので、また尋ねる。
func (s *Service) DeclineTypedPassword(typed terminal.TypedPassword) error {
	s.saveMutex.Lock()
	defer s.saveMutex.Unlock()
	graph, err := s.resolve()
	if err != nil {
		return err
	}
	identity := s.connectionIdentity(graph, typed.Alias)
	if identity.IsZero() {
		return nil
	}
	stored, precondition, err := s.metadata.Load()
	if err != nil {
		return err
	}
	index := hostMetadataIndex(stored.Hosts, identity)
	if index < 0 {
		stored.Hosts = append(stored.Hosts, HostMetadata{Identity: identity})
		index = len(stored.Hosts) - 1
	}
	stored.Hosts[index].PasswordSaveDeclinedBinding = typed.Binding
	_, err = s.commitMetadata(stored, precondition, "connection.decline-typed-password")
	return err
}
