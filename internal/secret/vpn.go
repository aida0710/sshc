package secret

import (
	"errors"
	"fmt"

	"sshc/internal/storage"
)

// VPN プロファイルの秘密は、資格情報の画面と API に現れない名前空間に置く。
// host にも鍵にも割り当てないので、割り当ての仕組みを通らない。
//
// 記録はプロファイルひとつにつき一件である。中身の形を決めるのは internal/vpn で、
// ここは封をして運ぶだけである。

// ErrUnknownVPNSecretsMutation は、知らない種類の変更を断る。
var ErrUnknownVPNSecretsMutation = errors.New("unknown vpn secrets mutation")

// VPNSecretsMutationKind は、VPN プロファイルの秘密への変更の種類である。
type VPNSecretsMutationKind string

const (
	// VPNSecretsSet は、プロファイルの記録を Document で置き換える。同じ名前の
	// 記録が残っていても引き継がない。
	VPNSecretsSet VPNSecretsMutationKind = "set"
	// VPNSecretsRename は、Profile の記録を NewName へ移す。
	VPNSecretsRename VPNSecretsMutationKind = "rename"
	// VPNSecretsRemove は、Profile の記録を消す。
	VPNSecretsRemove VPNSecretsMutationKind = "remove"
)

// VPNSecretsMutation は、プロファイルひとつぶんの秘密への変更である。
type VPNSecretsMutation struct {
	Kind    VPNSecretsMutationKind
	Profile string
	// NewName は、VPNSecretsRename のときの移し先である。
	NewName string
	// Document は、VPNSecretsSet のときの記録の本文である。
	Document string
}

// VPNSecrets は、プロファイルひとつぶんの秘密を返す。
func (s *Service) VPNSecrets(profile string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault := s.use()
	if vault == nil {
		return "", ErrLocked
	}
	value, ok := vault.Secret(KindVPN, profile)
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrUnknownCredential, profile)
	}
	return value, nil
}

// WithVPNSecretsTransaction は、秘密への変更を封じた vault の差し替えを作り、
// 呼び手が metadata の変更と同じ storage.Request で書けるようにする。
//
// vault がロック中なら、何も書かずに ErrLocked を返す。メモリ上の vault は、
// commit が成功したときだけ差し替わる。秘密が変わらないときは、commit に nil を渡す。
func (s *Service) WithVPNSecretsTransaction(
	mutation VPNSecretsMutation,
	commit func(vaultChange *storage.Change) (storage.Result, error),
) (storage.Result, error) {
	return s.commitVaultTransaction(vaultTransaction{
		apply: func(vault, clone *Vault) (bool, error) {
			return applyVPNSecretsMutation(vault, clone, mutation)
		},
		// vault が一度も作られていなければ、改名や削除で動かす記録も無い。
		commitsWithoutVault: mutation.Kind != VPNSecretsSet,
		commit:              commit,
	})
}

func applyVPNSecretsMutation(vault, clone *Vault, mutation VPNSecretsMutation) (bool, error) {
	current, exists := vault.Secret(KindVPN, mutation.Profile)
	switch mutation.Kind {
	case VPNSecretsSet:
		if exists && current == mutation.Document {
			return false, nil
		}
		return true, clone.Set(KindVPN, mutation.Profile, mutation.Document)
	case VPNSecretsRename:
		if mutation.Profile == mutation.NewName {
			return false, nil
		}
		// 移し先の名前に前の版が残した記録があれば捨てる。残すと、秘密を持たない
		// プロファイルを改名したときに、無関係な秘密を黙って引き継ぐ。
		_, leftover := vault.Secret(KindVPN, mutation.NewName)
		if leftover {
			if err := clone.Delete(KindVPN, mutation.NewName); err != nil {
				return false, err
			}
		}
		if !exists {
			return leftover, nil
		}
		return true, clone.RenameCredential(KindVPN, mutation.Profile, mutation.NewName)
	case VPNSecretsRemove:
		if !exists {
			return false, nil
		}
		return true, clone.Delete(KindVPN, mutation.Profile)
	default:
		return false, ErrUnknownVPNSecretsMutation
	}
}
