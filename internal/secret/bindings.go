package secret

import (
	"crypto/subtle"
	"errors"
	"slices"

	"sshc/internal/storage"
)

// PasswordMutationKind は、接続作成が vault に行う変更を表す。専用パスワードと
// 名前付き資格情報は保存構造が異なるため、単なるオプションフィールドの組ではなく
// 判別可能な種類として運ぶ。
type PasswordMutationKind string

const (
	PasswordMutationDedicated PasswordMutationKind = "dedicated_password"
	PasswordMutationSaved     PasswordMutationKind = "saved_password"
	PasswordMutationNewShared PasswordMutationKind = "new_shared_password"
	PasswordMutationRebind    PasswordMutationKind = "confirm_route"
	PasswordMutationRemove    PasswordMutationKind = "remove"
)

// PasswordMutation は接続 alias に一つのパスワード源を割り当てる要求である。
// Password はこのパッケージの外へ返らず、commit callback に渡す storage.Change にも
// 暗号化された bytes としてしか現れない。
type PasswordMutation struct {
	Kind       PasswordMutationKind
	Alias      string
	Credential string
	Password   string
	Binding    string
}

// KeyPassphraseMutation replaces the unlock value owned by one private-key
// path. Unlike a named credential, it cannot be reused by another key.
type KeyPassphraseMutation struct {
	RelativePath string
	Passphrase   string
}

// TOTPMutationKind は接続aliasに対するTOTPの割り当て変更である。
type TOTPMutationKind string

const (
	TOTPMutationSaved  TOTPMutationKind = "saved_totp"
	TOTPMutationRebind TOTPMutationKind = "confirm_route"
	TOTPMutationRemove TOTPMutationKind = "remove"
)

// AuthenticationBindingState describes whether a saved authentication value
// is available for the route currently resolved for its connection. It never
// exposes the value itself.
type AuthenticationBindingState string

const (
	AuthenticationBindingUnavailable AuthenticationBindingState = "unavailable"
	AuthenticationBindingNone        AuthenticationBindingState = "none"
	AuthenticationBindingCurrent     AuthenticationBindingState = "current"
	AuthenticationBindingStale       AuthenticationBindingState = "stale"
)

// TOTPMutation binds one saved TOTP seed to the authentication destination
// resolved for an alias. Provisioning data never leaves the vault transaction.
type TOTPMutation struct {
	Kind       TOTPMutationKind
	Alias      string
	Credential string
	Binding    string
}

// ConnectionSecretsMutation groups every vault change made by one connection
// save so callers can commit one sealed replacement beside the SSH config.
type ConnectionSecretsMutation struct {
	Password      *PasswordMutation
	KeyPassphrase *KeyPassphraseMutation
	TOTP          *TOTPMutation
}

// BoundPasswordFor returns a password only if current resolved destination is
// identical to the destination confirmed when the password was assigned.
func (s *Service) BoundPasswordFor(alias, binding string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault := s.use()
	if vault == nil {
		return ""
	}
	value, _ := vault.BoundPasswordFor(alias, binding)
	return value
}

// BoundTOTPFor returns TOTP provisioning data only while the host's resolved
// authentication destination still matches the assignment confirmation.
func (s *Service) BoundTOTPFor(alias, binding string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault := s.use()
	if vault == nil {
		return ""
	}
	value, _ := vault.BoundTOTPFor(alias, binding)
	return value
}

// HasTOTPFor reports whether an unlocked vault has a TOTP assignment without
// releasing its provisioning data.
func (s *Service) HasTOTPFor(alias string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault := s.use()
	if vault == nil {
		return false
	}
	_, ok := vault.SecretFor(KindTOTP, alias)
	return ok
}

// HasPasswordFor reports whether an unlocked vault has an account-password
// assignment for alias without releasing the secret. Callers use this to tell
// a missing assignment from one whose authentication binding became stale.
func (s *Service) HasPasswordFor(alias string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault := s.use()
	if vault == nil {
		return false
	}
	_, ok := vault.SecretFor(KindPassword, alias)
	return ok
}

// AuthenticationBindingStates reports password and TOTP assignment state for
// one resolved destination without releasing either saved value. Reading this
// metadata does not extend the vault idle deadline.
func (s *Service) AuthenticationBindingStates(alias, binding string) (AuthenticationBindingState, AuthenticationBindingState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault := s.open()
	if vault == nil {
		return AuthenticationBindingUnavailable, AuthenticationBindingUnavailable
	}
	state := func(kind Kind, bindings map[string]string) AuthenticationBindingState {
		if _, ok := vault.SecretFor(kind, alias); !ok {
			return AuthenticationBindingNone
		}
		if stored, ok := bindings[alias]; ok && stored == binding {
			return AuthenticationBindingCurrent
		}
		return AuthenticationBindingStale
	}
	return state(KindPassword, vault.passwordBindings), state(KindTOTP, vault.totpBindings)
}

// KeyPassphraseFor は、鍵のワークスペース相対パスを、保存済みのパスフレーズへ
// 解決する。鍵を二段階ではなく一度の操作でエージェントへ追加できるのはこれの
// おかげであり、鍵 vault が import するのではなく、そこへ注入される。
func (s *Service) KeyPassphraseFor(relativePath string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault := s.use()
	if vault == nil {
		return "", false
	}
	return vault.SecretFor(KindKeyPassphrase, relativePath)
}

// RelocateKeyPassphrases は鍵のパス変更に名前付きパスフレーズの割り当てを
// 追従させる。秘密の値には触れず、vault 内の subject 参照だけを一度に移す。
func (s *Service) RelocateKeyPassphrases(relocations map[string]string) error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	s.mu.Lock()
	vault := s.use()
	if vault == nil {
		s.mu.Unlock()
		return ErrLocked
	}
	clone := vault.clone()
	published := false
	defer func() {
		if !published {
			clone.Destroy()
		}
	}()
	changed, err := clone.RelocateSubjects(KindKeyPassphrase, relocations)
	baseline := slices.Clone(s.baseline)
	s.mu.Unlock()
	if err != nil || !changed {
		return err
	}
	if len(baseline) == 0 {
		return ErrNoVault
	}
	sealed, err := clone.Seal()
	if err != nil {
		return err
	}
	_, err = s.transactions.Commit(storage.Request{
		Operation: "secret.key-passphrase-relocate",
		Changes: []storage.Change{{
			Path: s.path(), Contents: sealed,
			Precondition: storage.Precondition{Exists: true, Digest: storage.Digest(baseline)},
		}},
	})
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.vault.Destroy()
	s.vault = clone
	published = true
	s.baseline = slices.Clone(sealed)
	s.used = s.now()
	s.mu.Unlock()
	return nil
}

// WithPasswordMutation prepares a password-vault replacement and lets the
// application commit it beside the SSH configuration change. The live vault
// remains unchanged until that callback succeeds. mutationMu stays held so a
// second writer cannot overtake the transaction; mu is deliberately released
// while storage runs because storage seals generational backups through this
// same service.
func (s *Service) WithPasswordMutation(
	mutation PasswordMutation,
	commit func(storage.Change) (storage.Result, error),
) (storage.Result, error) {
	return s.WithConnectionSecretsMutation(ConnectionSecretsMutation{Password: &mutation}, commit)
}

// WithConnectionSecretsMutation prepares one encrypted vault replacement for
// all secret changes belonging to a connection save. Disk and live memory are
// published only after the caller's combined storage transaction succeeds.
func (s *Service) WithConnectionSecretsMutation(
	mutation ConnectionSecretsMutation,
	commit func(storage.Change) (storage.Result, error),
) (storage.Result, error) {
	return s.WithConnectionSecretsTransaction(mutation, func(change *storage.Change) (storage.Result, error) {
		if change == nil {
			if mutation.Password != nil && mutation.Password.Kind == PasswordMutationRemove {
				return storage.Result{}, ErrNoPassword
			}
			return storage.Result{}, ErrNoPasswordMutation
		}
		return commit(*change)
	})
}

// WithConnectionSecretsTransaction keeps vault writers serialized across a
// connection commit even when the requested cleanup is a semantic no-op. A nil
// change lets the caller commit config without needlessly sealing the vault.
func (s *Service) WithConnectionSecretsTransaction(
	mutation ConnectionSecretsMutation,
	commit func(*storage.Change) (storage.Result, error),
) (storage.Result, error) {
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
		if !exists {
			passwordOnlyRemoval := mutation.Password != nil && mutation.Password.Kind == PasswordMutationRemove &&
				mutation.KeyPassphrase == nil && mutation.TOTP == nil
			totpOnlyRemoval := mutation.TOTP != nil && mutation.TOTP.Kind == TOTPMutationRemove &&
				mutation.Password == nil && mutation.KeyPassphrase == nil
			if passwordOnlyRemoval || totpOnlyRemoval {
				return commit(nil)
			}
			return storage.Result{}, ErrNoVault
		}
		return storage.Result{}, ErrLocked
	}
	clone := vault.clone()
	published := false
	defer func() {
		if !published {
			clone.Destroy()
		}
	}()
	changed := false
	if mutation.Password != nil {
		passwordChanged, err := applyPasswordMutation(vault, clone, *mutation.Password)
		if errors.Is(err, ErrNoPassword) && mutation.Password.Kind == PasswordMutationRemove {
			err = nil
			passwordChanged = false
		}
		if err != nil {
			s.mu.Unlock()
			return storage.Result{}, err
		}
		changed = changed || passwordChanged
	}
	if mutation.KeyPassphrase != nil {
		keyMutation := mutation.KeyPassphrase
		current, hasDedicated := vault.dedicatedKeyPassphrases[keyMutation.RelativePath]
		keyChanged := !hasDedicated || len(current) != len(keyMutation.Passphrase) ||
			subtle.ConstantTimeCompare([]byte(current), []byte(keyMutation.Passphrase)) != 1
		if keyChanged {
			if err := clone.SetDedicatedKeyPassphrase(keyMutation.RelativePath, keyMutation.Passphrase); err != nil {
				s.mu.Unlock()
				return storage.Result{}, err
			}
			changed = true
		}
	}
	if mutation.TOTP != nil {
		totpChanged, err := applyTOTPMutation(vault, clone, *mutation.TOTP)
		if err != nil {
			s.mu.Unlock()
			return storage.Result{}, err
		}
		changed = changed || totpChanged
	}
	if !changed {
		s.mu.Unlock()
		return commit(nil)
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
	result, err := commit(&change)
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

// WithStableSnapshot prevents vault/settings writers and master-key rotation
// from crossing a remote snapshot. The callback may then take the workspace
// mutation lock; this is the same mutationMu -> workspace order used by secret
// transactions and rekey.
func (s *Service) WithStableSnapshot(snapshot func() error) error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	if snapshot == nil {
		return nil
	}
	return snapshot()
}

func applyPasswordMutation(vault, clone *Vault, mutation PasswordMutation) (bool, error) {
	if mutation.Kind != PasswordMutationRemove && !validAuthenticationBinding(mutation.Binding) {
		return false, ErrPasswordBindingRequired
	}
	switch mutation.Kind {
	case PasswordMutationDedicated:
		if current, ok := vault.dedicatedPasswords[mutation.Alias]; ok &&
			len(current) == len(mutation.Password) &&
			subtle.ConstantTimeCompare([]byte(current), []byte(mutation.Password)) == 1 &&
			vault.passwordBindings[mutation.Alias] == mutation.Binding {
			return false, nil
		}
		if err := clone.SetDedicatedPassword(mutation.Alias, mutation.Password); err != nil {
			return false, err
		}
		if err := clone.BindPassword(mutation.Alias, mutation.Binding); err != nil {
			return false, err
		}
		return true, nil
	case PasswordMutationSaved:
		if current, ok := vault.Assigned(KindPassword, mutation.Alias); ok &&
			current == mutation.Credential && vault.passwordBindings[mutation.Alias] == mutation.Binding {
			return false, nil
		}
		if err := clone.Assign(KindPassword, mutation.Alias, mutation.Credential); err != nil {
			return false, err
		}
		if err := clone.BindPassword(mutation.Alias, mutation.Binding); err != nil {
			return false, err
		}
		return true, nil
	case PasswordMutationNewShared:
		if _, exists := vault.Secret(KindPassword, mutation.Credential); exists {
			return false, ErrCredentialAlreadyExists
		}
		if err := clone.Set(KindPassword, mutation.Credential, mutation.Password); err != nil {
			return false, err
		}
		if err := clone.Assign(KindPassword, mutation.Alias, mutation.Credential); err != nil {
			return false, err
		}
		if err := clone.BindPassword(mutation.Alias, mutation.Binding); err != nil {
			return false, err
		}
		return true, nil
	case PasswordMutationRebind:
		if _, ok := vault.SecretFor(KindPassword, mutation.Alias); !ok {
			return false, ErrNoPassword
		}
		if vault.passwordBindings[mutation.Alias] == mutation.Binding {
			return false, nil
		}
		if err := clone.BindPassword(mutation.Alias, mutation.Binding); err != nil {
			return false, err
		}
		return true, nil
	case PasswordMutationRemove:
		if _, ok := clone.SecretFor(KindPassword, mutation.Alias); !ok {
			return false, ErrNoPassword
		}
		clone.RemoveDedicatedPassword(mutation.Alias)
		clone.Unassign(KindPassword, mutation.Alias)
		return true, nil
	default:
		return false, ErrUnknownPasswordMutation
	}
}

func applyTOTPMutation(vault, clone *Vault, mutation TOTPMutation) (bool, error) {
	if mutation.Kind != TOTPMutationRemove && !validAuthenticationBinding(mutation.Binding) {
		return false, ErrPasswordBindingRequired
	}
	switch mutation.Kind {
	case TOTPMutationSaved:
		if current, ok := vault.Assigned(KindTOTP, mutation.Alias); ok &&
			current == mutation.Credential && vault.totpBindings[mutation.Alias] == mutation.Binding {
			return false, nil
		}
		if err := clone.Assign(KindTOTP, mutation.Alias, mutation.Credential); err != nil {
			return false, err
		}
		if err := clone.BindTOTP(mutation.Alias, mutation.Binding); err != nil {
			return false, err
		}
		return true, nil
	case TOTPMutationRebind:
		if _, ok := vault.SecretFor(KindTOTP, mutation.Alias); !ok {
			return false, ErrUnknownCredential
		}
		if vault.totpBindings[mutation.Alias] == mutation.Binding {
			return false, nil
		}
		if err := clone.BindTOTP(mutation.Alias, mutation.Binding); err != nil {
			return false, err
		}
		return true, nil
	case TOTPMutationRemove:
		if _, ok := vault.SecretFor(KindTOTP, mutation.Alias); !ok {
			return false, nil
		}
		clone.Unassign(KindTOTP, mutation.Alias)
		return true, nil
	default:
		return false, ErrUnknownTOTPMutation
	}
}
