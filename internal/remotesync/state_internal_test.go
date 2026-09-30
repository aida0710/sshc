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

func TestReadStateTreatsAnUnusableBaseAsNeverSynchronized(t *testing.T) {
	tests := []struct {
		name string
		base Manifest
	}{
		{
			name: "a base of another schema",
			base: Manifest{
				SchemaVersion: SchemaVersion - 1,
				CreatedAt:     "2026-08-30T00:00:00Z", Origin: "older-origin",
				ParentRevision: strings.Repeat("a", 64), Message: "Base written by an older schema",
			},
		},
		{
			name: "a base whose entry has no mode",
			base: Manifest{
				SchemaVersion: SchemaVersion,
				CreatedAt:     "2026-08-30T00:00:00Z", Origin: "edited-origin",
				ParentRevision: strings.Repeat("a", 64), Message: "Base without a mode",
				Files: []Entry{{Path: "config", SHA256: strings.Repeat("b", 64)}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loaded := readStateWithBase(t, test.base)
			if loaded.Base != nil || loaded.ETag != "" {
				t.Fatalf("state with %s = %#v, want the never-synchronized state", test.name, loaded)
			}
		})
	}
}

// readStateWithBaseは、baseを基準に持つstateファイルを置き、readStateで読み直す。
func readStateWithBase(t *testing.T, base Manifest) state {
	t.Helper()
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// readStateはworkspaceだけを読むので、ほかのつなぎ込みは要らない。
	service := &Service{workspace: workspace}
	base.Revision, _ = RevisionFor(base)
	document, err := json.Marshal(state{
		SchemaVersion: stateSchemaVersion,
		ETag:          "stored-etag", Key: "workspace.tar.gz.enc",
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
	return loaded
}
