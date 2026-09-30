package vpnprofile_test

import (
	"sync"
	"testing"

	"sshc/internal/application"
	"sshc/internal/secret"
	"sshc/internal/storage"
	"sshc/internal/vpn"
	"sshc/internal/vpnprofile"
)

// interleavingVault は、最初の Vault の書き込みが直列の順番を得る直前に、別の保存を
// 割り込ませる。同じプロファイルの保存が2つ同時に届いた順序を固定する。
type interleavingVault struct {
	*secret.Service
	once   sync.Once
	during func()
}

func (vault *interleavingVault) WithVPNSecretsTransaction(
	mutation secret.VPNSecretsMutation,
	commit func(vaultChange *storage.Change) (storage.Result, error),
) (storage.Result, error) {
	vault.once.Do(vault.during)
	return vault.Service.WithVPNSecretsTransaction(mutation, commit)
}

func l2tpProfile() application.VPNProfile {
	return application.VPNProfile{
		Name: "lab", Backend: vpn.L2TPIPsec,
		L2TP: &application.L2TPProfile{Server: "vpn.example.jp", Username: "fixture"},
	}
}

// パスワードだけを直す保存と、事前共有鍵だけを直す保存が重なっても、どちらの変更も残る。
func TestTwoOverlappingUpdatesKeepBothSecrets(t *testing.T) {
	f := newFixture(t)
	f.create(t, l2tpProfile(), vpn.SecretsDocument{L2TPPassword: "old-password", IPsecPSK: "old-psk"})
	other := vpnprofile.New(vpnprofile.Dependencies{
		Configuration: f.config, Vault: f.vault, Routes: f.routes, Startup: vpnprofile.NoStartupRebinder{},
	})
	var otherErr error
	vault := &interleavingVault{Service: f.vault, during: func() {
		otherErr = other.Update(l2tpProfile(), &vpn.SecretsDocument{IPsecPSK: "new-psk"})
	}}
	first := vpnprofile.New(vpnprofile.Dependencies{
		Configuration: f.config, Vault: vault, Routes: f.routes, Startup: vpnprofile.NoStartupRebinder{},
	})

	firstErr := first.Update(l2tpProfile(), &vpn.SecretsDocument{L2TPPassword: "new-password"})

	if otherErr != nil || firstErr != nil {
		t.Fatalf("Update = %v, %v", firstErr, otherErr)
	}
	if stored := f.storedSecrets(t, "lab"); stored.L2TPPassword != "new-password" || stored.IPsecPSK != "new-psk" {
		t.Fatalf("password = %q, psk = %q", stored.L2TPPassword, stored.IPsecPSK)
	}
}
