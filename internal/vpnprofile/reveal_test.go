package vpnprofile_test

import (
	"errors"
	"testing"

	"sshc/internal/application"
	"sshc/internal/secret"
	"sshc/internal/vpn"
)

// 取り出すのは、プロファイルの方式が使うシークレットだけである。
func TestRevealingReturnsOnlyTheSecretsOfTheBackend(t *testing.T) {
	f := newFixture(t)
	f.create(t, officeProfile(), vpn.SecretsDocument{OpenConnectPassword: "a password"})

	revealed, err := f.profiles.RevealSecrets("lab")

	if err != nil || revealed != (vpn.SecretsDocument{OpenConnectPassword: "a password"}) {
		t.Fatalf("RevealSecrets = %+v, %v", revealed, err)
	}
}

// Vault がロック中なら取り出さない。無いプロファイルの秘密も取り出さない。
func TestRevealingIsRefusedWhileTheVaultIsLockedOrTheProfileIsUnknown(t *testing.T) {
	f := newFixture(t)
	f.create(t, labProfile(), labSecrets())

	if _, err := f.profiles.RevealSecrets("missing"); !errors.Is(err, application.ErrUnknownVPNProfile) {
		t.Fatalf("RevealSecrets(missing) = %v", err)
	}
	f.vault.Lock()
	if revealed, err := f.profiles.RevealSecrets("lab"); !errors.Is(err, secret.ErrLocked) || revealed.WireGuardConfig != "" {
		t.Fatalf("RevealSecrets = %+v, %v, want ErrLocked", revealed, err)
	}
	if _, err := f.profiles.SecretsEvidence("lab"); !errors.Is(err, secret.ErrLocked) {
		t.Fatalf("SecretsEvidence = %v, want ErrLocked", err)
	}
}

// 確認のトークンに結び付ける要約は、秘密が書き換わると変わる。
func TestTheRevealEvidenceFollowsTheStoredSecrets(t *testing.T) {
	f := newFixture(t)
	f.create(t, officeProfile(), vpn.SecretsDocument{OpenConnectPassword: "a password"})
	before, err := f.profiles.SecretsEvidence("lab")
	if err != nil {
		t.Fatal(err)
	}

	if err := f.profiles.Update(officeProfile(), &vpn.SecretsDocument{OpenConnectPassword: "another password"}); err != nil {
		t.Fatal(err)
	}

	after, err := f.profiles.SecretsEvidence("lab")
	if err != nil || after == before {
		t.Fatalf("SecretsEvidence = %q, %v (before %q)", after, err, before)
	}
}
