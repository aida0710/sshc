package secret

import "fmt"

// VPN プロファイルの秘密は、資格情報の画面と API に現れない名前空間に置く。
// host にも鍵にも割り当てないので、割り当ての仕組みを通らない。
//
// 記録はプロファイルひとつにつき一件である。中身の形を決めるのは internal/vpn で、
// ここは封をして運ぶだけである。

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

// SetVPNSecrets は、プロファイルひとつぶんの秘密を置き換える。
func (s *Service) SetVPNSecrets(profile, document string) error {
	return s.mutateVault(func(vault *Vault) error { return vault.Set(KindVPN, profile, document) })
}

// RemoveVPNSecrets は、プロファイルの秘密を忘れる。
func (s *Service) RemoveVPNSecrets(profile string) error {
	return s.mutateVault(func(vault *Vault) error { return vault.Delete(KindVPN, profile) })
}
