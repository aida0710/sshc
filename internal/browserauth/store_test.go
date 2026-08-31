package browserauth_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"sshc/internal/browserauth"
	"sshc/internal/platform/windowsacl/acltest"
	"sshc/internal/storage"
)

func newStore(t *testing.T, random []byte) *browserauth.Store {
	t.Helper()
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return browserauth.NewStore(workspace, bytes.NewReader(random))
}

func TestRegisterStoresOnlyAHashAndVerifiesAfterRestart(t *testing.T) {
	store := newStore(t, bytes.Repeat([]byte{0x51}, 32))
	if err := store.SetPort(55447); err != nil {
		t.Fatal(err)
	}
	token, issued, err := store.Register("")
	if err != nil {
		t.Fatal(err)
	}
	if !issued || len(token) != 43 || !store.Verify(token) {
		t.Fatalf("issued=%t token length=%d verified=%t", issued, len(token), store.Verify(token))
	}
	contents, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(contents, []byte(token)) {
		t.Fatal("browser token was stored in plaintext")
	}
	if fresh, issued, err := store.Register(token); err != nil || issued || fresh != "" {
		t.Fatalf("existing registration = (%q, %t, %v)", fresh, issued, err)
	}
	if registered, err := store.HasRegistrations(); err != nil || !registered {
		t.Fatalf("HasRegistrations = (%t, %v)", registered, err)
	}
	if port, err := store.Port(); err != nil || port != 55447 {
		t.Fatalf("Port = (%d, %v)", port, err)
	}
}

func TestInvalidDocumentIsNotSilentlyReplaced(t *testing.T) {
	store := newStore(t, bytes.Repeat([]byte{0x52}, 32))
	if err := os.MkdirAll(filepath.Dir(store.Path()), 0o700); err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"schemaVersion":1,"hashes":[],"unknown":true}`)
	acltest.WritePrivateFile(t, store.Path(), original)
	if _, _, err := store.Register(""); !errors.Is(err, browserauth.ErrInvalidDocument) {
		t.Fatalf("Register = %v, want ErrInvalidDocument", err)
	}
	contents, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(contents, original) {
		t.Fatal("invalid registration document was overwritten")
	}
}
