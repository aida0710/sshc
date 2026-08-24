package recent_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sshc/internal/recent"
	"sshc/internal/storage"
)

func newStore(t *testing.T, moments ...time.Time) (*recent.Store, *storage.Workspace) {
	t.Helper()
	home := t.TempDir()
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	index := 0
	store := recent.NewStore(workspace, func() time.Time {
		moment := moments[min(index, len(moments)-1)]
		index++
		return moment
	})
	return store, workspace
}

func TestRecordKeepsTheNewestUseOfEachAlias(t *testing.T) {
	first := time.Date(2026, 8, 24, 1, 0, 0, 0, time.UTC)
	store, _ := newStore(t, first, first.Add(time.Minute), first.Add(2*time.Minute))
	for _, alias := range []string{"bastion", "database", "bastion"} {
		if err := store.Record(alias); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Alias != "bastion" || entries[1].Alias != "database" {
		t.Fatalf("entries = %#v", entries)
	}
	if entries[0].LastConnectedAt != "2026-08-24T01:02:00Z" {
		t.Fatalf("last connection = %q", entries[0].LastConnectedAt)
	}
}

func TestRecordKeepsOnlyTheNewestTwentyAliases(t *testing.T) {
	base := time.Date(2026, 8, 24, 1, 0, 0, 0, time.UTC)
	moments := make([]time.Time, recent.MaxEntries+2)
	for index := range moments {
		moments[index] = base.Add(time.Duration(index) * time.Minute)
	}
	store, _ := newStore(t, moments...)
	for index := range moments {
		if err := store.Record("host-" + string(rune('A'+index))); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != recent.MaxEntries || entries[0].Alias != "host-V" || entries[len(entries)-1].Alias != "host-C" {
		t.Fatalf("entries = %#v", entries)
	}
}

func TestAStoredDocumentIsPrivateAndAReplacementLeavesNoTemporaryFile(t *testing.T) {
	store, workspace := newStore(t, time.Now(), time.Now())
	if err := store.Record("bastion"); err != nil {
		t.Fatal(err)
	}
	if err := store.Record("database"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("mode = %o, want no group or other access", info.Mode().Perm())
	}
	children, err := os.ReadDir(workspace.StateDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, child := range children {
		if child.Name() != filepath.Base(store.Path()) {
			t.Errorf("unexpected state file %q", child.Name())
		}
	}
}

func TestRecordDoesNotOverwriteAnInvalidDocument(t *testing.T) {
	store, workspace := newStore(t, time.Now())
	if err := workspace.EnsureDirectory(workspace.StateDir()); err != nil {
		t.Fatal(err)
	}
	invalid := []byte(`{"schemaVersion":1,"entries":[{"alias":"bastion","lastConnectedAt":"not-a-time"}]}`)
	if err := os.WriteFile(store.Path(), invalid, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Record("database"); !errors.Is(err, recent.ErrInvalidDocument) {
		t.Fatalf("Record = %v, want ErrInvalidDocument", err)
	}
	contents, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != string(invalid) {
		t.Fatalf("invalid document was overwritten: %s", contents)
	}
}
