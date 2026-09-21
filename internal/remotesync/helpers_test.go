package remotesync_test

import "sshc/internal/remotesync"

// keyOf は、テストが手に持つ passphrase を製品と同じ KeyProvider の形で渡す。
func keyOf(passphrase string) remotesync.KeyProvider {
	return func() (string, error) { return passphrase, nil }
}

// replacing は ReplaceKeyUsing に、今の鍵と保存の手続きを渡す。
func replacing(currentKey string, commit func() error) remotesync.KeyReplacementProvider {
	return func() (string, func() error, error) { return currentKey, commit, nil }
}
