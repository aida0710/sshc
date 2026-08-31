package remotesync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshc/internal/platform/windowsacl/acltest"
	"sshc/internal/storage"
)

func TestRegisteredSnapshotMigrationsCoverEveryVersion(t *testing.T) {
	if snapshotMigrationBaseVersion > SchemaVersion {
		t.Fatalf("migration base %d is newer than schema %d", snapshotMigrationBaseVersion, SchemaVersion)
	}
	for version := snapshotMigrationBaseVersion; version < SchemaVersion; version++ {
		if registeredSnapshotMigrations[version] == nil {
			t.Errorf("snapshot schema %d -> %d has no registered migration", version, version+1)
		}
	}
	for version, step := range registeredSnapshotMigrations {
		if version < snapshotMigrationBaseVersion || version >= SchemaVersion || step == nil {
			t.Errorf("registered snapshot migration %d -> %d is outside the supported chain", version, version+1)
		}
	}
}

func TestReadStateMigratesItsV5Base(t *testing.T) {
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(workspace, nil, nil, nil)
	parent := strings.Repeat("a", 64)
	base := Manifest{
		SchemaVersion: 5,
		CreatedAt:     "2026-08-30T00:00:00Z", Origin: "v5-origin",
		ParentRevision: parent, Message: "Existing v5 state",
	}
	base.Revision, _ = RevisionFor(base)
	document, err := json.Marshal(state{
		SchemaVersion: stateSchemaVersion,
		ETag:          "v5-etag", Key: "workspace.tar.gz.enc",
		Base: &base, LastOperation: &SyncOperation{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(service.statePath()), storage.DirectoryPermission); err != nil {
		t.Fatal(err)
	}
	acltest.WritePrivateFile(t, service.statePath(), document)

	loaded, err := service.readState()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Base == nil || loaded.Base.SchemaVersion != SchemaVersion ||
		loaded.Base.Revision != base.Revision || len(loaded.Base.Ancestors) != 1 ||
		loaded.Base.Ancestors[0] != parent {
		t.Fatalf("migrated state base = %#v", loaded.Base)
	}
}
