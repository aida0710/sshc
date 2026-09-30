package syncrestore

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sshc/internal/remotesync"
	"sshc/internal/secret"
	"sshc/internal/storage"
)

const testPassphrase = "restore-test-passphrase"

var savedSettings = secret.SyncSettings{
	Endpoint: "https://s3.example.invalid", Bucket: "saved-bucket", Path: "team", Region: "auto",
	AccessKeyID: "AKIAEXAMPLE", SecretAccessKey: "s3cret-key", Direction: "both",
}

func newServices(t *testing.T) (*remotesync.Service, *secret.Service) {
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
	vault := secret.NewService(workspace, manager, time.Now)
	service, err := remotesync.NewIntegratedService(workspace, manager,
		func() string { return "2026-09-30T00:00:00Z" },
		func() (string, error) { return "origin-test", nil },
		remotesync.IntegrationHooks{
			OpenVault:          vault.TravelDocument,
			SealVault:          vault.AdoptTravelDocument,
			EmptyVaultDocument: vault.EmptyTravelDocument,
			VaultAdopted:       vault.Reload,
			KeyedTravelDigest:  vault.KeyedTravelDigest,
			OpenSnippets:       func() ([]byte, error) { return nil, nil },
			SealSnippets:       func(document []byte) ([]byte, error) { return document, nil },
			SecretMutation:     func(run func() error) error { return run() },
			StableSnapshot:     func(run func() error) error { return run() },
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := vault.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	if err := vault.SetSyncSettings(savedSettings); err != nil {
		t.Fatal(err)
	}
	return service, vault
}

func TestAnUnlockedVaultConfiguresSyncFromTheSavedSettings(t *testing.T) {
	service, vault := newServices(t)

	FromVault(service, vault, nil)

	if !service.Configured() {
		t.Fatal("sync is not configured from the saved settings")
	}
	endpoint, bucket, path, region := service.Target()
	if endpoint != savedSettings.Endpoint || bucket != savedSettings.Bucket || path != savedSettings.Path || region != savedSettings.Region {
		t.Fatalf("target = %q %q %q %q", endpoint, bucket, path, region)
	}
}

func TestALockedVaultLeavesSyncUnconfigured(t *testing.T) {
	service, vault := newServices(t)
	vault.Lock()

	FromVault(service, vault, nil)

	if service.Configured() {
		t.Fatal("sync was configured while the vault was locked")
	}
}

func TestSavedSettingsDoNotReplaceAnExplicitConfiguration(t *testing.T) {
	service, vault := newServices(t)
	explicit := remotesync.Config{
		Endpoint: "https://explicit.example.invalid", Bucket: "explicit-bucket", Region: "auto",
		Direction: remotesync.DirectionBoth,
	}
	credentials := remotesync.Credentials{AccessKeyID: "explicit", SecretAccessKey: "explicit-key"}
	if _, err := service.ConfigureIfUnconfigured(explicit, credentials, remotesync.NewClient(explicit, credentials)); err != nil {
		t.Fatal(err)
	}

	FromVault(service, vault, nil)

	if _, bucket, _, _ := service.Target(); bucket != "explicit-bucket" {
		t.Fatalf("bucket = %q, want the explicit one", bucket)
	}
}
