package vpnprofile_test

import (
	"errors"
	"strings"
	"testing"

	"sshc/internal/application"
	"sshc/internal/secret"
	"sshc/internal/storage"
	"sshc/internal/vpn"
)

// fieldsProfile は、v0.40.0 までの項目の形で保存した WireGuard のプロファイルである。
func fieldsProfile() application.VPNProfile {
	return application.VPNProfile{
		Name: "lab", Backend: vpn.WireGuard, DNS: []string{"10.9.9.53"},
		WireGuard: &application.WireGuardProfile{
			Servers: []string{"vpn.example.jp"}, Server: "vpn.example.jp:51820",
			PeerPublicKey: testPublicKey, Address: "10.9.9.2/32",
		},
	}
}

// saveFieldsProfile は、v0.40.0 が書いたのと同じ形で、項目の形のプロファイルと、秘密鍵だけの
// Vault の記録を書く。保存の手順（vpnprofile.Service）は項目を書かないので、metadata を直接書く。
func (f fixture) saveFieldsProfile(t *testing.T) {
	t.Helper()
	store := application.NewMetadataStore(f.workspace)
	metadata, precondition, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	metadata.VPNProfiles = append(metadata.VPNProfiles, fieldsProfile())
	metadataChange, err := store.Change(metadata, precondition)
	if err != nil {
		t.Fatal(err)
	}
	record := `{"wireguardPrivateKey":"` + testPrivateKey + `"}`
	if _, err := f.vault.WithVPNSecretsTransaction(secret.VPNSecretsMutation{
		Kind: secret.VPNSecretsSet, Profile: "lab", Document: record,
	}, func(vaultChange *storage.Change) (storage.Result, error) {
		return f.transactions.Commit(storage.Request{
			Operation: "test.fields", Changes: []storage.Change{metadataChange, *vaultChange},
		})
	}); err != nil {
		t.Fatal(err)
	}
}

// fieldsConfig は、fieldsProfile と Vault の秘密鍵から組み立てる設定ファイルである。
func fieldsConfig() string {
	fields, _ := fieldsProfile().WireGuardFields()
	return fields.Config(testPrivateKey)
}

// 項目の形のプロファイルは、保存し直すまで項目のまま使える。経路には、項目と秘密鍵から
// 組み立てた設定ファイルを渡す。
func TestAFieldsProfileStartsWithAConfigBuiltFromItsFields(t *testing.T) {
	f := newFixture(t)
	f.saveFieldsProfile(t)

	profile, secrets, err := f.profiles.Route("lab")

	if err != nil {
		t.Fatalf("Route = %v", err)
	}
	if secrets.WireGuard == nil || secrets.WireGuard.Config != fieldsConfig() {
		t.Fatalf("secrets = %+v", secrets.WireGuard)
	}
	if err := profile.ValidateSecrets(secrets); err != nil {
		t.Fatalf("ValidateSecrets = %v\n%s", err, secrets.WireGuard.Config)
	}
}

// 編集で開くと、項目と秘密鍵から組み立てた設定ファイルを、鍵を含めて返す。
func TestRevealingAFieldsProfileShowsTheBuiltConfig(t *testing.T) {
	f := newFixture(t)
	f.saveFieldsProfile(t)

	revealed, err := f.profiles.RevealSecrets("lab")

	if err != nil || revealed.WireGuardConfig != fieldsConfig() || !strings.Contains(revealed.WireGuardConfig, testPrivateKey) {
		t.Fatalf("RevealSecrets = %+v, %v", revealed, err)
	}
}

// 設定を直さずに保存し直すと、組み立てた設定ファイルを Vault に書き、項目と秘密鍵を消す。
func TestSavingAFieldsProfileMovesItToAConfig(t *testing.T) {
	f := newFixture(t)
	f.saveFieldsProfile(t)
	listed, err := f.config.StoredVPNProfile("lab")
	if err != nil {
		t.Fatal(err)
	}

	if err := f.profiles.Update(listed, nil); err != nil {
		t.Fatalf("Update = %v", err)
	}

	saved, err := f.config.StoredVPNProfile("lab")
	if err != nil {
		t.Fatal(err)
	}
	if _, found := saved.WireGuardFields(); found {
		t.Fatalf("項目が残った: %+v", saved.WireGuard)
	}
	record, err := f.vault.VPNSecrets("lab")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(record, "wireguardPrivateKey") {
		t.Fatalf("秘密鍵の項目が残った: %s", record)
	}
	if got := f.storedSecrets(t, "lab"); got.WireGuardConfig != fieldsConfig() {
		t.Fatalf("stored = %+v", got)
	}
	// 移したあとも、同じ設定ファイルで経路を起こせる。
	if _, secrets, err := f.profiles.Route("lab"); err != nil || secrets.WireGuard.Config != fieldsConfig() {
		t.Fatalf("Route = %+v, %v", secrets.WireGuard, err)
	}
}

// 編集した設定ファイルを送って保存すると、その設定ファイルになる。
func TestSavingAFieldsProfileWithAnEditedConfigUsesTheEditedConfig(t *testing.T) {
	f := newFixture(t)
	f.saveFieldsProfile(t)
	edited := strings.Replace(fieldsConfig(), "AllowedIPs = 0.0.0.0/0", "AllowedIPs = 10.9.0.0/16", 1)
	profile := fieldsProfile()
	profile.WireGuard = &application.WireGuardProfile{Servers: []string{"vpn.example.jp"}}

	if err := f.profiles.Update(profile, &vpn.SecretsDocument{WireGuardConfig: edited}); err != nil {
		t.Fatalf("Update = %v", err)
	}

	if got := f.storedSecrets(t, "lab"); got.WireGuardConfig != edited {
		t.Fatalf("stored = %+v", got)
	}
}

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
