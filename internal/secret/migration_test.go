package secret

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sshc/internal/envelope"
	"sshc/internal/storage"
)

const migrationTestPassphrase = "a migration test password"

func TestRegisteredMigrationsCoverEveryVersionFromTheSupportedBase(t *testing.T) {
	if migrationBaseVersion > SchemaVersion {
		t.Fatalf("migration base %d is newer than schema %d", migrationBaseVersion, SchemaVersion)
	}
	for version := migrationBaseVersion; version < SchemaVersion; version++ {
		if registeredDocumentMigrations[version] == nil {
			t.Errorf("schema %d -> %d has no registered migration", version, version+1)
		}
	}
	for version, step := range registeredDocumentMigrations {
		if version < migrationBaseVersion || version >= SchemaVersion || step == nil {
			t.Errorf("registered migration %d -> %d is outside the supported chain", version, version+1)
		}
	}
}

func TestSchemaFourFixtureRemainsMigratableAndReadable(t *testing.T) {
	fixture, err := os.ReadFile("testdata/schema-v4.json")
	if err != nil {
		t.Fatal(err)
	}
	vault, _, err := openDocumentWithMigrations(fixture, envelope.Key{}, registeredDocumentMigrations)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := vault.BoundPasswordFor(
		"bastion",
		"abababababababababababababababababababababababababababababababab",
	); !ok || got != "fixture-shared-password" {
		t.Fatalf("migrated fixture password = %q, %v", got, ok)
	}
	if got, ok := vault.SecretFor(KindKeyPassphrase, "keys/team.pem"); !ok || got != "fixture-shared-key-passphrase" {
		t.Fatalf("migrated fixture key passphrase = %q, %v", got, ok)
	}
}

func TestDocumentMigrationsRunOneVersionAtATime(t *testing.T) {
	seen := []int{}
	migrated, migration, err := migrateDocument(
		[]byte(`{"schemaVersion":2,"passwords":{},"keyPassphrases":{},"hosts":{},"keys":{}}`),
		migrationRegistry{
			2: func(fields map[string]json.RawMessage) error {
				seen = append(seen, 2)
				fields["introducedByThree"] = json.RawMessage(`true`)
				return nil
			},
			3: func(fields map[string]json.RawMessage) error {
				seen = append(seen, 3)
				if !bytes.Equal(fields["introducedByThree"], []byte(`true`)) {
					return errors.New("the previous migration did not run")
				}
				return nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if migration != (Migration{From: 2, To: SchemaVersion}) {
		t.Fatalf("migration = %+v", migration)
	}
	if len(seen) != 2 || seen[0] != 2 || seen[1] != 3 {
		t.Fatalf("migration order = %v", seen)
	}
	_, version, err := migrationFields(migrated)
	if err != nil || version != SchemaVersion {
		t.Fatalf("migrated schema = %d, %v", version, err)
	}
}

func TestDocumentMigrationRequiresAnUnbrokenPath(t *testing.T) {
	original := []byte(`{"schemaVersion":2,"passwords":{},"keyPassphrases":{},"hosts":{},"keys":{}}`)
	_, _, err := migrateDocument(original, migrationRegistry{2: func(map[string]json.RawMessage) error { return nil }})
	var version *SchemaVersionError
	if !errors.As(err, &version) || version.Found != 2 || version.Supported != SchemaVersion {
		t.Fatalf("migrateDocument = %v", err)
	}
}

func TestDocumentMigrationReportsTheFailingStepWithoutChangingInput(t *testing.T) {
	original := []byte(`{"schemaVersion":3,"passwords":{},"keyPassphrases":{},"hosts":{},"keys":{}}`)
	before := bytes.Clone(original)
	cause := errors.New("injected migration failure")
	_, _, err := migrateDocument(original, migrationRegistry{
		3: func(map[string]json.RawMessage) error { return cause },
	})
	var migration *MigrationError
	if !errors.Is(err, ErrMigrationFailed) || !errors.Is(err, cause) ||
		!errors.As(err, &migration) || migration.From != 3 || migration.To != 4 {
		t.Fatalf("migrateDocument = %v", err)
	}
	if !bytes.Equal(original, before) {
		t.Fatal("migration changed its input buffer")
	}
}

func TestDocumentMigrationClassifiesInvalidFinalShapeAsAMigrationFailure(t *testing.T) {
	plaintext := []byte(`{"schemaVersion":3,"passwords":{},"keyPassphrases":{},"hosts":{},"keys":{}}`)
	_, _, err := openDocumentWithMigrations(plaintext, envelope.Key{}, migrationRegistry{
		3: func(fields map[string]json.RawMessage) error {
			fields["fieldUnknownToCurrentSchema"] = json.RawMessage(`true`)
			return nil
		},
	})
	var migration *MigrationError
	if !errors.Is(err, ErrMigrationFailed) || !errors.As(err, &migration) ||
		migration.From != 3 || migration.To != SchemaVersion {
		t.Fatalf("openDocumentWithMigrations = %v", err)
	}
}

func TestUnlockCommitsAMigrationAndKeepsTheEncryptedPreviousGeneration(t *testing.T) {
	service, _, manager, vaultPath, original := migrationHarness(t, storage.OSFileSystem{})
	service.migrations = migrationRegistry{3: initialisePasswordBindings}
	manager.Seal = service.SealBackup
	manager.Unseal = service.OpenBackup

	if err := service.Unlock(migrationTestPassphrase); err != nil {
		t.Fatal(err)
	}
	onDisk, err := os.ReadFile(vaultPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(onDisk, original) {
		t.Fatal("migration did not replace the old vault")
	}
	if _, err := Open(onDisk, migrationTestPassphrase); err != nil {
		t.Fatalf("committed vault is not current: %v", err)
	}
	state, err := service.State()
	if err != nil || state.LastMigration != (Migration{From: 3, To: SchemaVersion}) {
		t.Fatalf("state after migration = %+v, %v", state, err)
	}
	history, err := manager.History()
	if err != nil || len(history) != 1 || history[0].Operation != "secret.migrate-vault" {
		t.Fatalf("history = %#v, %v", history, err)
	}
	wrapped, err := os.ReadFile(filepath.Join(history[0].BackupDir, filepath.FromSlash(WorkspacePath)))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(wrapped, original) {
		t.Fatal("previous vault was copied into history without backup encryption")
	}
	previous, err := service.OpenBackup(wrapped)
	if err != nil || !bytes.Equal(previous, original) {
		t.Fatalf("encrypted previous generation = %d bytes, %v", len(previous), err)
	}
}

func TestUnlockLeavesTheOriginalVaultAndMemoryLockedWhenMigrationCommitFails(t *testing.T) {
	injected := errors.New("injected migration commit failure")
	failing := &migrationRenameFailure{FileSystem: storage.OSFileSystem{}, failure: injected}
	service, _, manager, vaultPath, original := migrationHarness(t, failing)
	service.migrations = migrationRegistry{3: initialisePasswordBindings}
	manager.Seal = service.SealBackup
	failing.enabled = true

	if err := service.Unlock(migrationTestPassphrase); !errors.Is(err, injected) {
		t.Fatalf("Unlock = %v", err)
	}
	if service.Unlocked() {
		t.Fatal("failed migration remained open in memory")
	}
	onDisk, err := os.ReadFile(vaultPath)
	if err != nil || !bytes.Equal(onDisk, original) {
		t.Fatalf("vault after rollback = %d bytes, %v", len(onDisk), err)
	}
}

func TestUnlockPublishesAMigratedVaultOnlyAfterTheDiskCommitPoint(t *testing.T) {
	blocking := &migrationBlockingRename{
		FileSystem: storage.OSFileSystem{},
		entered:    make(chan struct{}),
		release:    make(chan struct{}),
	}
	service, _, manager, vaultPath, _ := migrationHarness(t, blocking)
	blocking.target = vaultPath
	service.migrations = migrationRegistry{3: initialisePasswordBindings}
	manager.Seal = service.SealBackup

	result := make(chan error, 1)
	go func() { result <- service.Unlock(migrationTestPassphrase) }()
	select {
	case <-blocking.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("migration did not reach its disk commit point")
	}
	if service.Unlocked() {
		t.Fatal("migrated vault became visible before the disk commit point")
	}
	close(blocking.release)
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("migration did not finish after the disk commit point")
	}
	if !service.Unlocked() {
		t.Fatal("committed migrated vault was not published")
	}
}

func initialisePasswordBindings(fields map[string]json.RawMessage) error {
	fields["passwordBindings"] = json.RawMessage(`{}`)
	return nil
}

func migrationHarness(
	t *testing.T,
	fileSystem storage.FileSystem,
) (*Service, *storage.Workspace, *storage.Manager, string, []byte) {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh", "sshc"), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(fileSystem, home)
	if err != nil {
		t.Fatal(err)
	}
	manager := storage.NewManager(workspace, time.Now, rand.Reader)
	service := NewService(workspace, manager, time.Now)
	key, err := envelope.Derive(migrationTestPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	original, err := key.Seal([]byte(`{"schemaVersion":3,"passwords":{},"keyPassphrases":{},"hosts":{},"keys":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	vaultPath := filepath.Join(workspace.Root(), filepath.FromSlash(WorkspacePath))
	if err := os.WriteFile(vaultPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	return service, workspace, manager, vaultPath, original
}

type migrationRenameFailure struct {
	storage.FileSystem
	failure error
	enabled bool
}

type migrationBlockingRename struct {
	storage.FileSystem
	target  string
	entered chan struct{}
	release chan struct{}
}

func (f *migrationBlockingRename) Rename(oldPath, newPath string) error {
	if newPath == f.target {
		close(f.entered)
		<-f.release
	}
	return f.FileSystem.Rename(oldPath, newPath)
}

func (f *migrationRenameFailure) Rename(oldPath, newPath string) error {
	if f.enabled && filepath.Base(newPath) == filepath.Base(WorkspacePath) {
		return f.failure
	}
	return f.FileSystem.Rename(oldPath, newPath)
}
