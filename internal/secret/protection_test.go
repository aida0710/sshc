package secret_test

import (
	"bytes"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sshc/internal/envelope"
	"sshc/internal/secret"
	"sshc/internal/storage"
)

func TestProtectionModeRoundTripPreservesSecretsAndSettings(t *testing.T) {
	s, workspace, manager := recoveryService(t)
	if err := s.Initialise("1234"); err != nil {
		t.Fatal(err)
	}
	if err := setTestPassword(s, "server", "credential-canary"); err != nil {
		t.Fatal(err)
	}
	settings := secret.SyncSettings{Key: "strong-independent-sync-key", SecretAccessKey: "storage-canary"}
	if err := s.SetSyncSettings(settings); err != nil {
		t.Fatal(err)
	}
	docPath := filepath.Join(workspace.StateDir(), "protected-test")
	if err := s.RegisterProtectedDocument(secret.ProtectedDocument{Path: docPath, Validate: func([]byte) error { return nil }}); err != nil {
		t.Fatal(err)
	}
	sealed, err := s.SealDocument([]byte("snippet-canary"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Commit(storage.Request{Operation: "test.document", Changes: []storage.Change{{Path: docPath, Contents: sealed}}}); err != nil {
		t.Fatal(err)
	}
	if err := s.ChangeMasterPassword("wrong", ""); !errors.Is(err, secret.ErrWrongPassphrase) {
		t.Fatalf("wrong password: %v", err)
	}
	if err := s.ChangeMasterPassword("1234", ""); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(workspace.Root(), secret.LocalKeyPath)
	body, err := os.ReadFile(keyPath)
	if err != nil || len(body) != 32 {
		t.Fatalf("local key: length=%d, err=%v", len(body), err)
	}
	defer clear(body)
	s.Lock()
	restarted := secret.NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader), time.Now)
	if err := restarted.AutoUnlock(); err != nil {
		t.Fatal(err)
	}
	state, err := restarted.State()
	if err != nil || !state.Unlocked || !state.Passwordless {
		t.Fatalf("state: %+v, %v", state, err)
	}
	if got := restarted.BoundPasswordFor("server", testAuthenticationBinding); got != "credential-canary" {
		t.Fatalf("credential lost: %q", got)
	}
	if got, err := restarted.SyncSettings(); err != nil || got != settings {
		t.Fatalf("settings: %+v, %v", got, err)
	}
	sealed, err = os.ReadFile(docPath)
	if err != nil {
		t.Fatal(err)
	}
	if plain, err := restarted.OpenDocument(sealed); err != nil || string(plain) != "snippet-canary" {
		t.Fatalf("document: %v", err)
	}
	// Rotate with the original service, which owns the protected-document registry.
	if err := s.Unlock(""); err != nil {
		t.Fatal(err)
	}
	if err := s.ChangeMasterPassword("", "5678"); err != nil {
		t.Fatal(err)
	}
	marker, err := os.ReadFile(keyPath)
	if err != nil || len(marker) != 0 {
		t.Fatalf("device key retained: %v", err)
	}
	s.Lock()
	if err := s.AutoUnlock(); err != nil || s.Unlocked() {
		t.Fatalf("password vault auto-unlocked: %v", err)
	}
	if err := s.Unlock(""); !errors.Is(err, secret.ErrWrongPassphrase) {
		t.Fatalf("empty unlock: %v", err)
	}
	if err := s.Unlock("5678"); err != nil {
		t.Fatal(err)
	}
	if got, err := s.SyncSettings(); err != nil || got != settings {
		t.Fatalf("settings after password: %+v, %v", got, err)
	}
	// No old device key survives in durable journals, history or generation backups.
	if err := filepath.WalkDir(workspace.StateDir(), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(contents, body) {
			t.Errorf("old device key retained in %s", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPasswordlessCreationSkipsIdleLockButExplicitLockDestroysKey(t *testing.T) {
	s, _ := newService(t)
	if err := s.Initialise(""); err != nil {
		t.Fatal(err)
	}
	s.SetIdleTimeout(time.Nanosecond)
	if !s.Unlocked() {
		t.Fatal("passwordless vault idle-locked")
	}
	s.Lock()
	if s.Unlocked() {
		t.Fatal("explicit lock did not destroy key")
	}
	if err := s.AutoUnlock(); err != nil || !s.Unlocked() {
		t.Fatalf("auto unlock: %v", err)
	}
	if _, err := envelope.Derive("1234"); !errors.Is(err, envelope.ErrWeakPassphrase) {
		t.Fatalf("sync policy weakened: %v", err)
	}
	if _, err := secret.Create("abc"); !errors.Is(err, secret.ErrWeakPassphrase) {
		t.Fatalf("three characters accepted: %v", err)
	}
}

func TestRemovingPasswordRollsBackProtectionAndDocumentsOnFailure(t *testing.T) {
	service, _, faults, _, before := newRekeyFaultHarness(t)
	faults.failRenames = map[int]bool{2: true}
	if err := service.ChangeMasterPassword(passphrase, ""); !errors.Is(err, faults.failure) {
		t.Fatalf("ChangeMasterPassword = %v", err)
	}
	assertRekeyGeneration(t, service, before, passphrase, "")
}

func TestStartupRecoversProtectionSwitchAtEitherSideOfCommitPoint(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(map[bool]string{false: "before commit", true: "after commit"}[committed], func(t *testing.T) {
			service, _, faults, home, _ := newRekeyFaultHarness(t)
			if committed {
				faults.failCleanupSync = true
			} else {
				faults.failRenames = map[int]bool{2: true, 3: true}
			}
			err := service.ChangeMasterPassword(passphrase, "")
			if committed && err != nil {
				t.Fatal(err)
			}
			if !committed && !errors.Is(err, faults.failure) {
				t.Fatalf("failure not injected: %v", err)
			}
			faults.failCleanupSync = false
			faults.failRenames = nil
			workspace, err := storage.NewWorkspace(faults, home)
			if err != nil {
				t.Fatal(err)
			}
			manager := storage.NewManager(workspace, time.Now, rand.Reader)
			restarted := secret.NewService(workspace, manager, time.Now)
			if err := restarted.AutoUnlock(); err != nil {
				t.Fatal(err)
			}
			if got := restarted.Unlocked(); got != committed {
				t.Fatalf("unlocked = %v", got)
			}
			if !committed {
				if err := restarted.Unlock(passphrase); err != nil {
					t.Fatal(err)
				}
			}
			pending, err := manager.Pending()
			if err != nil || len(pending) != 0 {
				t.Fatalf("pending: %+v, %v", pending, err)
			}
		})
	}
}

func TestPasswordlessSchemaResetPreservesAutomaticOpeningMode(t *testing.T) {
	service, workspace, _ := recoveryService(t)
	if err := service.Initialise(""); err != nil {
		t.Fatal(err)
	}
	sealed, err := service.SealDocument([]byte(`{"schemaVersion":999}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace.Root(), secret.WorkspacePath), sealed, 0600); err != nil {
		t.Fatal(err)
	}
	service.Lock()
	restarted := secret.NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader), time.Now)
	if err := restarted.AutoUnlock(); !errors.Is(err, secret.ErrNewerSchema) {
		t.Fatalf("AutoUnlock = %v", err)
	}
	if err := restarted.ResetUnsupported(""); err != nil {
		t.Fatal(err)
	}
	restarted.SetIdleTimeout(time.Nanosecond)
	if !restarted.Unlocked() {
		t.Fatal("reset passwordless vault incorrectly idle-locked")
	}
}
