package secret

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"time"

	"sshc/internal/storage"
	"sshc/internal/totp"
)

// mutateVault prepares a private candidate and publishes it only after the
// encrypted replacement is durable. The baseline belongs to the vault which
// was cloned; using it as the precondition also prevents another process from
// being overwritten after this service unlocked the document.
func (s *Service) mutateVault(mutate func(*Vault) error) error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()

	s.mu.Lock()
	vault := s.use()
	if vault == nil {
		s.mu.Unlock()
		return ErrLocked
	}
	clone := vault.clone()
	baseline := slices.Clone(s.baseline)
	s.mu.Unlock()

	published := false
	defer func() {
		if !published {
			clone.Destroy()
		}
	}()
	if err := mutate(clone); err != nil {
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
		Operation: "secret.vault",
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
	s.mu.Unlock()
	return nil
}

// SetBound stores a dedicated password and records the resolved authentication
// destination that may receive it.
func (s *Service) SetBound(alias, password, binding string) error {
	if !validAuthenticationBinding(binding) {
		return ErrPasswordBindingRequired
	}
	return s.mutateVault(func(vault *Vault) error {
		if err := vault.SetDedicatedPassword(alias, password); err != nil {
			return err
		}
		return vault.Bind(KindPassword, alias, binding)
	})
}

// Rename は、保存済みのパスワードを新しい alias へ引き継ぐ。ホストの名前変更が
// それを置き去りにすれば、二度と誰も尋ねない名前の下にパスワードが残る。
func (s *Service) Rename(from, to string) error {
	return s.mutateVault(func(vault *Vault) error {
		if err := vault.Rename(KindPassword, from, to); err != nil {
			return err
		}
		return vault.Rename(KindTOTP, from, to)
	})
}

// Credentials は、両方の種別のすべての資格情報名と、その使用先を列挙する。
//
// 名前と使用先だけで、値は決して返さない。これは各画面が読むものであり、秘密を
// 読める画面があれば、それは侵害されたブラウザがそこから秘密を読める画面だという
// ことになる。
func (s *Service) Credentials() (map[Kind]map[string][]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault := s.open()
	if vault == nil {
		return nil, ErrLocked
	}
	listed := map[Kind]map[string][]string{}
	for _, kind := range []Kind{KindPassword, KindKeyPassphrase, KindTOTP} {
		listed[kind] = map[string][]string{}
		for _, name := range vault.Names(kind) {
			uses := vault.Uses(kind, name)
			if uses == nil {
				uses = []string{}
			}
			listed[kind][name] = uses
		}
	}
	return listed, nil
}

// SetCredential は、資格情報を作るか、その値を置き換える。
//
// 置き換えは、共有された秘密をローテーションする方法である。その名前を指している
// すべての subject が新しい値を読む。名前が存在する理由そのものだ。
func (s *Service) SetCredential(kind Kind, name, value string) error {
	return s.mutateVault(func(vault *Vault) error { return vault.Set(kind, name, value) })
}

// Credential は、明示的な表示・編集操作に限って名前付き資格情報の値を返す。
// 一覧や割り当て確認は Credentials を使い、この境界を通らない。
func (s *Service) Credential(kind Kind, name string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault := s.use()
	if vault == nil {
		return "", ErrLocked
	}
	if !ValidKind(kind) {
		return "", ErrUnknownKind
	}
	value, ok := vault.Secret(kind, name)
	if !ok {
		return "", ErrUnknownCredential
	}
	return value, nil
}

// CredentialEvidence は、一度限りの表示確認を現在の資格情報へ結び付ける。
// 値そのものを session manager に保持せず、名前・種別・現在値のダイジェストだけを渡す。
func (s *Service) CredentialEvidence(kind Kind, name string) (string, error) {
	value, err := s.Credential(kind, name)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(string(kind) + "\x00" + name + "\x00" + value))
	return hex.EncodeToString(digest[:]), nil
}

// TOTPCodeSet contains the adjacent codes needed to tolerate a clock boundary
// without releasing the provisioning secret from the engine process.
type TOTPCodeSet struct {
	Previous         string
	Current          string
	Next             string
	PeriodSeconds    int
	RemainingSeconds int
}

// TOTPCodes generates the previous, current, and next code for one named TOTP
// credential. The encrypted provisioning value stays inside the service.
func (s *Service) TOTPCodes(name string, at time.Time) (TOTPCodeSet, error) {
	value, err := s.Credential(KindTOTP, name)
	if err != nil {
		return TOTPCodeSet{}, err
	}
	configuration, err := totp.Parse(value)
	if err != nil {
		return TOTPCodeSet{}, ErrInvalidTOTP
	}
	period := time.Duration(configuration.Period) * time.Second
	previous, err := configuration.Code(at.Add(-period))
	if err != nil {
		return TOTPCodeSet{}, ErrInvalidTOTP
	}
	current, err := configuration.Code(at)
	if err != nil {
		return TOTPCodeSet{}, ErrInvalidTOTP
	}
	next, err := configuration.Code(at.Add(period))
	if err != nil {
		return TOTPCodeSet{}, ErrInvalidTOTP
	}
	remaining := configuration.Period - int(at.Unix()%int64(configuration.Period))
	return TOTPCodeSet{
		Previous: previous, Current: current, Next: next,
		PeriodSeconds: configuration.Period, RemainingSeconds: remaining,
	}, nil
}

// UpdateCredential は、名前と値を一つの vault 置換として更新する。
// 名前を参照している host / key は同じ commit 内で新しい名前へ追従する。
func (s *Service) UpdateCredential(kind Kind, currentName, nextName, value string) error {
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
	baseline := slices.Clone(s.baseline)
	s.mu.Unlock()

	if err := clone.RenameCredential(kind, currentName, nextName); err != nil {
		return err
	}
	if err := clone.Set(kind, nextName, value); err != nil {
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
		Operation: "secret.credential.update",
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
	s.mu.Unlock()
	return nil
}

// DeleteCredential は資格情報を忘れる。何かがそれを指しているあいだは拒否する。
func (s *Service) DeleteCredential(kind Kind, name string) error {
	return s.mutateVault(func(vault *Vault) error { return vault.Delete(kind, name) })
}

// AssignCredential は、subject を同じ種別の資格情報に向ける。種別が防護である。
// 他方の種別の名前が現れるマップは存在しない。
func (s *Service) AssignCredential(kind Kind, subject, name string) error {
	if kind == KindPassword || kind == KindTOTP {
		return ErrPasswordBindingRequired
	}
	return s.mutateVault(func(vault *Vault) error { return vault.Assign(kind, subject, name) })
}

// BoundAssignment は、経路に束縛される資格情報（パスワード・TOTP）を alias に
// 割り当てる要求。Binding は割り当てを確認したときの解決済み接続先。
type BoundAssignment struct {
	Kind    Kind
	Subject string
	Name    string
	Binding string
}

// AssignBoundCredential は名前付きの資格情報を、alias の現在の解決済み接続先に
// 束縛して割り当てる。パスワードと TOTP は同じ境界を共有する。
func (s *Service) AssignBoundCredential(assignment BoundAssignment) error {
	if !validAuthenticationBinding(assignment.Binding) {
		return ErrPasswordBindingRequired
	}
	return s.mutateVault(func(vault *Vault) error {
		if err := vault.Assign(assignment.Kind, assignment.Subject, assignment.Name); err != nil {
			return err
		}
		return vault.Bind(assignment.Kind, assignment.Subject, assignment.Binding)
	})
}

// UnassignCredential は subject の参照を忘れ、資格情報自体は残す。
func (s *Service) UnassignCredential(kind Kind, subject string) error {
	return s.mutateVault(func(vault *Vault) error {
		vault.Unassign(kind, subject)
		return nil
	})
}

// DedicatedKeyPassphrases returns only key paths with a non-reusable value.
// Locked vaults reveal no subjects.
func (s *Service) DedicatedKeyPassphrases() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault := s.open()
	if vault == nil {
		return nil
	}
	return vault.DedicatedKeyPassphraseSubjects()
}

// Aliases は、パスワードが保存されているホストを返す。ロック中は何も返さない。
func (s *Service) Aliases() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault := s.open()
	if vault == nil {
		return nil
	}
	return vault.Subjects(KindPassword)
}
