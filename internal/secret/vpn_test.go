package secret_test

import (
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sshc/internal/secret"
	"sshc/internal/storage"
)

const wireGuardRecord = `{"wireguardPrivateKey":"fixture"}`

// newVPNSecretsService は、vault を作った service と、呼び手の commit が使う
// storage manager を返す。
func newVPNSecretsService(t *testing.T) (*secret.Service, *storage.Manager) {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	manager := storage.NewManager(workspace, time.Now, rand.Reader)
	service := secret.NewService(workspace, manager, time.Now)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	return service, manager
}

// commitVault は、vault の変更だけを書く commit である。
func commitVault(manager *storage.Manager) func(*storage.Change) (storage.Result, error) {
	return func(change *storage.Change) (storage.Result, error) {
		if change == nil {
			return storage.Result{}, nil
		}
		return manager.Commit(storage.Request{Operation: "test.vpn", Changes: []storage.Change{*change}})
	}
}

func setVPNSecrets(t *testing.T, service *secret.Service, manager *storage.Manager, profile, document string) {
	t.Helper()
	if _, err := service.WithVPNSecretsTransaction(secret.VPNSecretsMutation{
		Kind: secret.VPNSecretsSet, Profile: profile, Document: document,
	}, commitVault(manager)); err != nil {
		t.Fatalf("set %s = %v", profile, err)
	}
}

// VPN の秘密は保存して読み出せ、消すと読めなくなる。
func TestVPNSecretsAreStoredAndReadBack(t *testing.T) {
	service, manager := newVPNSecretsService(t)
	setVPNSecrets(t, service, manager, "tohoku", wireGuardRecord)

	stored, err := service.VPNSecrets("tohoku")
	if err != nil || stored != wireGuardRecord {
		t.Fatalf("VPNSecrets = %q, %v", stored, err)
	}
	if _, err := service.WithVPNSecretsTransaction(secret.VPNSecretsMutation{
		Kind: secret.VPNSecretsRemove, Profile: "tohoku",
	}, commitVault(manager)); err != nil {
		t.Fatalf("remove = %v", err)
	}
	if _, err := service.VPNSecrets("tohoku"); !errors.Is(err, secret.ErrUnknownCredential) {
		t.Fatalf("VPNSecrets after removal = %v", err)
	}
}

// 改名すると秘密が新しい名前へ移り、移し先に残っていた記録は捨てられる。
func TestRenamingVPNSecretsReplacesALeftoverRecord(t *testing.T) {
	service, manager := newVPNSecretsService(t)
	setVPNSecrets(t, service, manager, "tohoku", wireGuardRecord)
	setVPNSecrets(t, service, manager, "kyushu", `{"wireguardPrivateKey":"leftover"}`)

	if _, err := service.WithVPNSecretsTransaction(secret.VPNSecretsMutation{
		Kind: secret.VPNSecretsRename, Profile: "tohoku", NewName: "kyushu",
	}, commitVault(manager)); err != nil {
		t.Fatalf("rename = %v", err)
	}
	if stored, err := service.VPNSecrets("kyushu"); err != nil || stored != wireGuardRecord {
		t.Fatalf("VPNSecrets(kyushu) = %q, %v", stored, err)
	}
	if _, err := service.VPNSecrets("tohoku"); !errors.Is(err, secret.ErrUnknownCredential) {
		t.Fatalf("VPNSecrets(tohoku) = %v", err)
	}
}

// 秘密を持たないプロファイルを改名しても、移し先に残っていた記録は引き継がない。
func TestRenamingAProfileWithoutSecretsDropsTheLeftoverRecord(t *testing.T) {
	service, manager := newVPNSecretsService(t)
	setVPNSecrets(t, service, manager, "kyushu", `{"wireguardPrivateKey":"leftover"}`)

	if _, err := service.WithVPNSecretsTransaction(secret.VPNSecretsMutation{
		Kind: secret.VPNSecretsRename, Profile: "tohoku", NewName: "kyushu",
	}, commitVault(manager)); err != nil {
		t.Fatalf("rename = %v", err)
	}
	if _, err := service.VPNSecrets("kyushu"); !errors.Is(err, secret.ErrUnknownCredential) {
		t.Fatalf("VPNSecrets(kyushu) = %v, want the leftover to be dropped", err)
	}
}

// ロック中は何も書かずに断り、commit を呼ばない。
func TestALockedVaultRefusesVPNSecretsWithoutCommitting(t *testing.T) {
	service, manager := newVPNSecretsService(t)
	setVPNSecrets(t, service, manager, "tohoku", wireGuardRecord)
	service.Lock()

	for _, kind := range []secret.VPNSecretsMutationKind{secret.VPNSecretsSet, secret.VPNSecretsRename, secret.VPNSecretsRemove} {
		committed := false
		_, err := service.WithVPNSecretsTransaction(secret.VPNSecretsMutation{
			Kind: kind, Profile: "tohoku", NewName: "kyushu", Document: wireGuardRecord,
		}, func(*storage.Change) (storage.Result, error) {
			committed = true
			return storage.Result{}, nil
		})
		if !errors.Is(err, secret.ErrLocked) || committed {
			t.Fatalf("%s = %v, committed %v; want ErrLocked without commit", kind, err, committed)
		}
	}
}

// commit が失敗したら、メモリ上の vault も変わらない。
func TestAFailedCommitLeavesTheVPNSecretsUnchanged(t *testing.T) {
	service, manager := newVPNSecretsService(t)
	setVPNSecrets(t, service, manager, "tohoku", wireGuardRecord)
	refused := errors.New("metadata write failed")

	_, err := service.WithVPNSecretsTransaction(secret.VPNSecretsMutation{
		Kind: secret.VPNSecretsRemove, Profile: "tohoku",
	}, func(change *storage.Change) (storage.Result, error) {
		if change == nil {
			t.Fatal("remove did not produce a vault change")
		}
		return storage.Result{}, refused
	})
	if !errors.Is(err, refused) {
		t.Fatalf("remove = %v, want the commit failure", err)
	}
	if stored, err := service.VPNSecrets("tohoku"); err != nil || stored != wireGuardRecord {
		t.Fatalf("VPNSecrets after the failed commit = %q, %v", stored, err)
	}
}

// 同じ本文を置き直しても vault は書かない。
func TestSettingTheSameVPNSecretsLeavesTheVaultUnwritten(t *testing.T) {
	service, manager := newVPNSecretsService(t)
	setVPNSecrets(t, service, manager, "tohoku", wireGuardRecord)

	_, err := service.WithVPNSecretsTransaction(secret.VPNSecretsMutation{
		Kind: secret.VPNSecretsSet, Profile: "tohoku", Document: wireGuardRecord,
	}, func(change *storage.Change) (storage.Result, error) {
		if change != nil {
			t.Fatal("an unchanged record was sealed again")
		}
		return storage.Result{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// VPN の秘密は、資格情報の画面と API の名前空間には現れない。
func TestVPNSecretsAreNotReachableThroughTheCredentialNamespaces(t *testing.T) {
	service, manager := newVPNSecretsService(t)
	setVPNSecrets(t, service, manager, "tohoku", wireGuardRecord)

	if _, err := service.Credential(secret.Kind("vpn"), "tohoku"); !errors.Is(err, secret.ErrUnknownKind) {
		t.Fatalf("Credential(vpn) = %v, want ErrUnknownKind", err)
	}
	listed, err := service.Credentials()
	if err != nil {
		t.Fatal(err)
	}
	for kind, names := range listed {
		if len(names) != 0 {
			t.Fatalf("資格情報の一覧に %s が %v として現れた", kind, names)
		}
	}
}
