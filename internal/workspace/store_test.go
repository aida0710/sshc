package workspace_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"sshc/internal/platform/windowsacl/acltest"
	"sshc/internal/storage"
	"sshc/internal/workspace"
)

func TestStorePersistsPrivateStateAndLeavesNoTemporaryFile(t *testing.T) {
	store := newStore(t)
	now := time.Date(2026, 8, 24, 7, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	for _, stored := range []workspace.Workspace{
		{ID: "older", Name: "Older", Layout: pane("one", "web"), FocusedPaneID: "one", CreatedAt: now, UpdatedAt: now},
		{ID: "newer", Name: "Newer", Layout: pane("two", "db"), FocusedPaneID: "two", CreatedAt: now, UpdatedAt: time.Date(2026, 8, 24, 8, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)},
	} {
		if err := store.Save(stored); err != nil {
			t.Fatal(err)
		}
	}
	listed, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 || listed[0].ID != "newer" || listed[1].ID != "older" {
		t.Fatalf("List = %#v", listed)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(store.Path())
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != storage.FilePermission {
			t.Fatalf("permission = %v", info.Mode().Perm())
		}
	}
	children, err := os.ReadDir(filepath.Dir(store.Path()))
	if err != nil {
		t.Fatal(err)
	}
	for _, child := range children {
		if child.Name() != filepath.Base(store.Path()) {
			t.Errorf("unexpected state file %q", child.Name())
		}
	}
}

func TestInvalidOrNewerDocumentsAreNotOverwritten(t *testing.T) {
	cases := map[string]struct {
		contents []byte
		want     error
	}{
		"newer":   {[]byte(`{"schemaVersion":2,"workspaces":[]}`), workspace.ErrUnsupportedSchema},
		"unknown": {[]byte(`{"schemaVersion":1,"workspaces":[],"surprise":true}`), workspace.ErrInvalidDocument},
		"invalid": {[]byte(`{"schemaVersion":1,"workspaces":[{"id":"bad"}]}`), workspace.ErrInvalidDocument},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			store := newStore(t)
			acltest.WritePrivateFile(t, store.Path(), test.contents)
			if _, err := store.List(); !errors.Is(err, test.want) {
				t.Fatalf("List = %v, want %v", err, test.want)
			}
			now := time.Now().UTC().Format(time.RFC3339Nano)
			err := store.Save(workspace.Workspace{
				ID: "new", Name: "New", Layout: pane("one", "web"), FocusedPaneID: "one",
				CreatedAt: now, UpdatedAt: now,
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("Save = %v, want %v", err, test.want)
			}
			contents, err := os.ReadFile(store.Path())
			if err != nil {
				t.Fatal(err)
			}
			if string(contents) != string(test.contents) {
				t.Fatalf("document was overwritten: %s", contents)
			}
		})
	}
}
