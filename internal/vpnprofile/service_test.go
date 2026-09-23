package vpnprofile_test

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sshc/internal/application"
	"sshc/internal/secret"
	"sshc/internal/storage"
	"sshc/internal/vpn"
	"sshc/internal/vpnprofile"
)

const (
	testPrivateKey  = "aAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAA="
	testPublicKey   = "bBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBA="
	vaultPassphrase = "a vpn profile test passphrase"
)

// recordedRoutes は、止めるよう頼まれた経路の名前を覚える。
type recordedRoutes struct {
	stopped []string
}

func (routes *recordedRoutes) Stop(_ context.Context, name string) error {
	routes.stopped = append(routes.stopped, name)
	return nil
}

// failingCommit は、metadata の書き込みだけを失敗させる。
type failingCommit struct {
	*application.Service
}

var errCommitRefused = errors.New("the storage transaction failed")

func (failingCommit) CommitVPNProfileChange(application.VPNProfileChange, *storage.Change) (application.SaveResult, error) {
	return application.SaveResult{}, errCommitRefused
}

type fixture struct {
	profiles *vpnprofile.Service
	config   *application.Service
	vault    *secret.Service
	routes   *recordedRoutes
	// transactions は、metadata を通さずに Vault だけを書くときに使う。
	transactions *storage.Manager
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	home := t.TempDir()
	root := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config"), []byte("Host lab\n  HostName 10.9.9.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	transactions := storage.NewManager(workspace, time.Now, rand.Reader)
	config := application.NewService(workspace, transactions)
	vault := secret.NewService(workspace, transactions, time.Now)
	if err := vault.Initialise(vaultPassphrase); err != nil {
		t.Fatal(err)
	}
	routes := &recordedRoutes{}
	return fixture{
		profiles: vpnprofile.New(vpnprofile.Dependencies{Configuration: config, Vault: vault, Routes: routes}),
		config:   config, vault: vault, routes: routes, transactions: transactions,
	}
}

func labProfile() application.VPNProfile {
	return application.VPNProfile{
		Name: "lab", Backend: string(vpn.WireGuard), Target: "10.9.9.1:22",
		WireGuard: &application.WireGuardProfile{
			Server: "vpn.example.jp:51820", PeerPublicKey: testPublicKey, Address: "10.9.9.2/32",
		},
	}
}

func officeProfile() application.VPNProfile {
	return application.VPNProfile{
		Name: "lab", Backend: string(vpn.OpenConnect), Target: "10.9.9.1:22",
		OpenConnect: &application.OpenConnectProfile{Server: "vpn.example.jp", Username: "fixture"},
	}
}

func (f fixture) create(t *testing.T, profile application.VPNProfile, secrets vpn.SecretsDocument) {
	t.Helper()
	if err := f.profiles.Create(profile, secrets); err != nil {
		t.Fatalf("Create = %v", err)
	}
}

func (f fixture) storedSecrets(t *testing.T, name string) vpn.SecretsDocument {
	t.Helper()
	stored, err := f.vault.VPNSecrets(name)
	if err != nil {
		t.Fatalf("VPNSecrets(%s) = %v", name, err)
	}
	secrets, err := vpn.DecodeSecrets(stored)
	if err != nil {
		t.Fatal(err)
	}
	return secrets.Document()
}

func (f fixture) profileNames(t *testing.T) []string {
	t.Helper()
	profiles, err := f.config.VPNProfiles()
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		names = append(names, profile.Name)
	}
	return names
}

// 作成は、同じ名前で Vault に残っていた秘密を引き継がない。
func TestCreatingAProfileDoesNotInheritALeftoverSecret(t *testing.T) {
	f := newFixture(t)
	// v0.38.0 は、ロック中の削除で秘密だけを Vault に残すことがあった。
	leftover := `{"openconnectPassword":"a password left by an older version"}`
	if _, err := f.vault.WithVPNSecretsTransaction(secret.VPNSecretsMutation{
		Kind: secret.VPNSecretsSet, Profile: "lab", Document: leftover,
	}, func(change *storage.Change) (storage.Result, error) {
		return f.transactions.Commit(storage.Request{Operation: "test.leftover", Changes: []storage.Change{*change}})
	}); err != nil {
		t.Fatal(err)
	}

	f.create(t, labProfile(), vpn.SecretsDocument{WireGuardPrivateKey: testPrivateKey})

	if got := f.storedSecrets(t, "lab"); got != (vpn.SecretsDocument{WireGuardPrivateKey: testPrivateKey}) {
		t.Fatalf("stored = %+v, want only the sent key", got)
	}
}

// 更新は、空の項目の秘密を保存済みのまま残す。
func TestUpdatingSettingsKeepsTheStoredSecrets(t *testing.T) {
	f := newFixture(t)
	f.create(t, officeProfile(), vpn.SecretsDocument{OpenConnectPassword: "a password"})
	updated := officeProfile()
	updated.OpenConnect.Username = "renamed-user"

	if err := f.profiles.Update(updated, &vpn.SecretsDocument{}); err != nil {
		t.Fatalf("Update = %v", err)
	}

	if got := f.storedSecrets(t, "lab"); got.OpenConnectPassword != "a password" {
		t.Fatalf("stored = %+v", got)
	}
}

// 方式を変えた更新は、前の方式の秘密を捨てる。
func TestChangingTheBackendDropsTheSecretsOfTheOldBackend(t *testing.T) {
	f := newFixture(t)
	f.create(t, officeProfile(), vpn.SecretsDocument{OpenConnectPassword: "a password"})

	if err := f.profiles.Update(labProfile(), &vpn.SecretsDocument{WireGuardPrivateKey: testPrivateKey}); err != nil {
		t.Fatalf("Update = %v", err)
	}

	if got := f.storedSecrets(t, "lab"); got != (vpn.SecretsDocument{WireGuardPrivateKey: testPrivateKey}) {
		t.Fatalf("stored = %+v, want only the wireguard key", got)
	}
}

// 方式を変えたのに新しい方式の秘密が無ければ、何も変えずに断る。
func TestChangingTheBackendWithoutItsSecretChangesNothing(t *testing.T) {
	f := newFixture(t)
	f.create(t, officeProfile(), vpn.SecretsDocument{OpenConnectPassword: "a password"})

	err := f.profiles.Update(labProfile(), nil)

	var fieldError *vpn.FieldError
	if !errors.As(err, &fieldError) || fieldError.Field != "secrets."+vpn.SecretKeyWireGuardPrivateKey {
		t.Fatalf("Update = %v, want the missing wireguard key", err)
	}
	profiles, _ := f.config.VPNProfiles()
	if len(profiles) != 1 || profiles[0].Backend != string(vpn.OpenConnect) {
		t.Fatalf("profiles = %+v", profiles)
	}
	if got := f.storedSecrets(t, "lab"); got.OpenConnectPassword != "a password" {
		t.Fatalf("stored = %+v", got)
	}
}

// 無いプロファイルは更新しない。
func TestUpdatingAnUnknownProfileIsRefused(t *testing.T) {
	f := newFixture(t)

	err := f.profiles.Update(labProfile(), &vpn.SecretsDocument{WireGuardPrivateKey: testPrivateKey})

	if !errors.Is(err, application.ErrUnknownVPNProfile) {
		t.Fatalf("Update = %v, want ErrUnknownVPNProfile", err)
	}
}

// ロック中の削除と改名は断り、経路も止めず、設定も秘密も変えない。
func TestALockedVaultRefusesRemovalAndRenameWithoutStoppingTheRoute(t *testing.T) {
	f := newFixture(t)
	f.create(t, labProfile(), vpn.SecretsDocument{WireGuardPrivateKey: testPrivateKey})
	f.vault.Lock()

	if err := f.profiles.Remove(context.Background(), "lab"); !errors.Is(err, secret.ErrLocked) {
		t.Fatalf("Remove = %v, want ErrLocked", err)
	}
	if err := f.profiles.Rename(context.Background(), "lab", "tains"); !errors.Is(err, secret.ErrLocked) {
		t.Fatalf("Rename = %v, want ErrLocked", err)
	}

	if len(f.routes.stopped) != 0 {
		t.Fatalf("断ったのに経路を止めた: %v", f.routes.stopped)
	}
	if names := f.profileNames(t); len(names) != 1 || names[0] != "lab" {
		t.Fatalf("profiles = %v", names)
	}
	if err := f.vault.Unlock(vaultPassphrase); err != nil {
		t.Fatal(err)
	}
	if got := f.storedSecrets(t, "lab"); got.WireGuardPrivateKey != testPrivateKey {
		t.Fatalf("stored = %+v", got)
	}
}

// 書き込みが失敗したら、設定も秘密も変わらない。
func TestAFailedCommitChangesNeitherTheProfileNorItsSecrets(t *testing.T) {
	f := newFixture(t)
	f.create(t, labProfile(), vpn.SecretsDocument{WireGuardPrivateKey: testPrivateKey})
	failing := vpnprofile.New(vpnprofile.Dependencies{
		Configuration: failingCommit{f.config}, Vault: f.vault, Routes: f.routes,
	})

	if err := failing.Remove(context.Background(), "lab"); !errors.Is(err, errCommitRefused) {
		t.Fatalf("Remove = %v, want the commit failure", err)
	}

	if names := f.profileNames(t); len(names) != 1 || names[0] != "lab" {
		t.Fatalf("profiles = %v", names)
	}
	if got := f.storedSecrets(t, "lab"); got.WireGuardPrivateKey != testPrivateKey {
		t.Fatalf("stored = %+v", got)
	}
}

// 改名の検査で断る場合は、使っている経路を止めない。
func TestARefusedRenameDoesNotStopTheRoute(t *testing.T) {
	f := newFixture(t)
	f.create(t, labProfile(), vpn.SecretsDocument{WireGuardPrivateKey: testPrivateKey})
	other := labProfile()
	other.Name = "office"
	f.create(t, other, vpn.SecretsDocument{WireGuardPrivateKey: testPrivateKey})

	for _, test := range []struct {
		to   string
		want error
	}{
		{"has space", vpn.ErrProfileName},
		{strings.Repeat("a", 49), vpn.ErrProfileName},
		{"office", application.ErrVPNProfileExists},
	} {
		if err := f.profiles.Rename(context.Background(), "lab", test.to); !errors.Is(err, test.want) {
			t.Fatalf("Rename(%q) = %v, want %v", test.to, err, test.want)
		}
	}
	if len(f.routes.stopped) != 0 {
		t.Fatalf("断ったのに経路を止めた: %v", f.routes.stopped)
	}
}

// 同じ名前への改名は何もしない。
func TestRenamingToTheSameNameDoesNothing(t *testing.T) {
	f := newFixture(t)
	f.create(t, labProfile(), vpn.SecretsDocument{WireGuardPrivateKey: testPrivateKey})

	if err := f.profiles.Rename(context.Background(), "lab", "lab"); err != nil {
		t.Fatalf("Rename = %v", err)
	}
	if len(f.routes.stopped) != 0 {
		t.Fatalf("何もしない改名で経路を止めた: %v", f.routes.stopped)
	}
}

// 改名は経路を止めてから、設定・秘密・接続の紐付けを一緒に移す。
func TestRenamingStopsTheRouteAndMovesEverythingTogether(t *testing.T) {
	f := newFixture(t)
	f.create(t, labProfile(), vpn.SecretsDocument{WireGuardPrivateKey: testPrivateKey})
	if _, err := f.config.SetConnectionVPN("lab", "lab"); err != nil {
		t.Fatal(err)
	}

	if err := f.profiles.Rename(context.Background(), "lab", "tains"); err != nil {
		t.Fatalf("Rename = %v", err)
	}

	if len(f.routes.stopped) != 1 || f.routes.stopped[0] != "lab" {
		t.Fatalf("stopped = %v", f.routes.stopped)
	}
	if bound, err := f.config.ConnectionVPN("lab"); err != nil || bound != "tains" {
		t.Fatalf("ConnectionVPN = %q, %v", bound, err)
	}
	if got := f.storedSecrets(t, "tains"); got.WireGuardPrivateKey != testPrivateKey {
		t.Fatalf("stored = %+v", got)
	}
}

// 削除は経路を止め、設定・紐付け・秘密を一緒に消す。
func TestRemovingStopsTheRouteAndForgetsEverything(t *testing.T) {
	f := newFixture(t)
	f.create(t, labProfile(), vpn.SecretsDocument{WireGuardPrivateKey: testPrivateKey})
	if _, err := f.config.SetConnectionVPN("lab", "lab"); err != nil {
		t.Fatal(err)
	}

	if err := f.profiles.Remove(context.Background(), "lab"); err != nil {
		t.Fatalf("Remove = %v", err)
	}

	if len(f.routes.stopped) != 1 || f.routes.stopped[0] != "lab" {
		t.Fatalf("stopped = %v", f.routes.stopped)
	}
	if names := f.profileNames(t); len(names) != 0 {
		t.Fatalf("profiles = %v", names)
	}
	if bound, err := f.config.ConnectionVPN("lab"); err != nil || bound != "" {
		t.Fatalf("ConnectionVPN = %q, %v", bound, err)
	}
	if _, err := f.vault.VPNSecrets("lab"); !errors.Is(err, secret.ErrUnknownCredential) {
		t.Fatalf("VPNSecrets = %v", err)
	}
}
