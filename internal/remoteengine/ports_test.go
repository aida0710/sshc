package remoteengine

import (
	"errors"
	"os"
	"testing"

	"sshc/internal/storage"
)

func newPortStore(t *testing.T) *PortStore {
	t.Helper()
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return NewPortStore(workspace)
}

func TestARememberedPortIsReturnedForItsAliasOnly(t *testing.T) {
	store := newPortStore(t)
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
	store := newPortStore(t)
	if err := store.Remember("r540", 61000); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.path(), []byte(`{"schemaVersion":1,"ports":{"r540":0}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Port("r540"); !errors.Is(err, ErrInvalidPorts) {
		t.Fatalf("Port = %v, want ErrInvalidPorts", err)
	}
}
