package app

import (
	"crypto/rand"
	"testing"

	"sshc/internal/remotesync"
)

// 同期先のアクセスキーとシークレットは Vault の中にある。Vault をロックしたら、
// 同期のサービスもそれを手放す。
func TestLockingTheVaultDropsTheSyncCredentials(t *testing.T) {
	services, err := newEngineServices(Dependencies{Home: t.TempDir(), Random: rand.Reader})
	if err != nil {
		t.Fatal(err)
	}
	if err := services.vault.Initialise("correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	config := remotesync.Config{
		Endpoint: "https://objects.example.test", Bucket: "sshc", Region: "auto", Direction: remotesync.DirectionBoth,
	}
	credentials := remotesync.Credentials{AccessKeyID: "AKIDEXAMPLE12345", SecretAccessKey: "secret"}
	if applied, err := services.sync.ConfigureIfUnconfigured(config, credentials, remotesync.NewClient(config, credentials)); err != nil || !applied {
		t.Fatalf("ConfigureIfUnconfigured = %v, %v", applied, err)
	}

	services.vault.Lock()
	if services.sync.Configured() {
		t.Fatal("the sync binding outlived the vault lock")
	}
	if suffix := services.sync.AccessKeySuffix(); suffix != "" {
		t.Fatalf("AccessKeySuffix after the lock = %q, want nothing", suffix)
	}
}
