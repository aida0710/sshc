package remoteengine

import (
	"errors"
	"os"
	"testing"

	"sshc/internal/storage"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return NewStore(workspace)
}

func TestARememberedPortIsReturnedForItsAliasOnly(t *testing.T) {
	store := newStore(t)
	if err := store.Remember("r540", 61000); err != nil {
		t.Fatal(err)
	}
	port, found, err := store.Port("r540")
	if err != nil || !found || port != 61000 {
		t.Fatalf("Port(r540) = %d, %v, %v", port, found, err)
	}
	if _, found, err := store.Port("other"); err != nil || found {
		t.Fatalf("Port(other) found = %v, %v", found, err)
	}
}

func TestABrokenPortsFileIsReportedRatherThanReplaced(t *testing.T) {
	store := newStore(t)
	if err := store.Remember("r540", 61000); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.path(), []byte(`{"schemaVersion":1,"ports":{"r540":0}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Port("r540"); !errors.Is(err, ErrInvalidStore) {
		t.Fatalf("Port = %v, want ErrInvalidStore", err)
	}
}

func TestAddedEnginesAreKeptInOrderAndRemovedWithoutTouchingPorts(t *testing.T) {
	store := newStore(t)
	for _, alias := range []string{"r540", "gpu-box", "r540"} {
		if err := store.AddEngine(alias); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Remember("r540", 61000); err != nil {
		t.Fatal(err)
	}
	if err := store.RemoveEngine("r540"); err != nil {
		t.Fatal(err)
	}
	engines, err := store.Engines()
	if err != nil || len(engines) != 1 || engines[0] != "gpu-box" {
		t.Fatalf("engines = %v, %v", engines, err)
	}
	if port, found, err := store.Port("r540"); err != nil || !found || port != 61000 {
		t.Fatalf("port after removing the engine = %d, %v, %v", port, found, err)
	}
}
