package application

import (
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"sshc/internal/platform/windowsacl/acltest"
	"sshc/internal/secret"
)

// sealingWithALockedVault は、マスターパスワードのある Vault がまだロックされているときの、
// 世代バックアップの封じ方である。
func sealingWithALockedVault([]byte) ([]byte, error) {
	return nil, secret.ErrLocked
}

func writeSchema9Files(t *testing.T, service *Service, stateDirectory string) (legacyPath string) {
	t.Helper()
	if err := service.metadata.EnsureDirectory(); err != nil {
		t.Fatal(err)
	}
	acltest.WritePrivateFile(t, service.metadata.Path(), []byte(metadataSchema9))
	legacyPath = filepath.Join(stateDirectory, legacyEngineSettingsFileName)
	acltest.WritePrivateFile(t, legacyPath, []byte(`{"schemaVersion":1,"port":50123}`))
	return legacyPath
}

func requireSchema10Metadata(t *testing.T, service *Service) {
	t.Helper()
	contents, err := os.ReadFile(service.metadata.Path())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeMetadata(contents); err != nil {
		t.Fatalf("metadata.json was not moved to schema %d: %v", MetadataSchemaVersion, err)
	}
}

// 起動の時点で Vault がロックされていて控えを封じられなくても、古い形のファイルを移して起動する。
func TestAMachineWhoseVaultIsLockedStillMovesItsOldFilesAtStart(t *testing.T) {
	service, workspace := newTerminalService(t)
	service.manager.Seal = sealingWithALockedVault
	legacyPath := writeSchema9Files(t, service, workspace.StateDir())

	if err := service.PrepareMachine(rand.Reader, testMachineStart); err != nil {
		t.Fatal(err)
	}

	requireSchema10Metadata(t, service)
	if port := service.ThisMachineSettings().Port; port != 50123 {
		t.Fatalf("port = %d, want 50123", port)
	}
	if _, err := os.Stat(legacyPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("engine-settings.json still exists: %v", err)
	}
}

// 控えを封じられずに止まった書き直しの記録が残っていても、それを片付けて書き直し、起動する。
func TestAMetadataMoveThatStoppedBeforeReplacingAnythingDoesNotBlockTheNextStart(t *testing.T) {
	service, workspace := newTerminalService(t)
	service.manager.Seal = sealingWithALockedVault
	writeSchema9Files(t, service, workspace.StateDir())
	// 控えを取る形で書き直そうとして、封じられずに止まる。
	migrated, err := migrateMetadataFromSchema9([]byte(metadataSchema9))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.commitMetadata(migrated, preconditionFor([]byte(metadataSchema9), true), metadataSchema10Operation); !errors.Is(err, secret.ErrLocked) {
		t.Fatalf("moving with a sealed backup = %v, want ErrLocked", err)
	}
	if pending, err := service.manager.Pending(); err != nil || len(pending) != 1 {
		t.Fatalf("pending = %#v, %v; want the stopped move", pending, err)
	}

	if err := service.PrepareMachine(rand.Reader, testMachineStart); err != nil {
		t.Fatal(err)
	}

	requireSchema10Metadata(t, service)
	if pending, err := service.manager.Pending(); err != nil || len(pending) != 0 {
		t.Fatalf("pending = %#v, %v; want none", pending, err)
	}
}

// 前のバージョンの engine-settings.json を読めなくても（持ち主の違うファイルなど）、ポートを移さずに
// 起動する。読めないファイルは中身を確かめられないので、消さずに残す。
func TestAnEngineSettingsFileThatCannotBeReadDoesNotStopTheStart(t *testing.T) {
	service, workspace := newTerminalService(t)
	legacyPath := writeSchema9Files(t, service, workspace.StateDir())
	// ファイルの代わりにディレクトリを置き、どの OS でも読めないようにする。
	if err := os.Remove(legacyPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(legacyPath, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := service.PrepareMachine(rand.Reader, testMachineStart); err != nil {
		t.Fatal(err)
	}

	requireSchema10Metadata(t, service)
	if port := service.ThisMachineSettings().Port; port != 0 {
		t.Fatalf("port = %d, want 0 (the machine's own port)", port)
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("the unreadable engine-settings.json was removed: %v", err)
	}
}
