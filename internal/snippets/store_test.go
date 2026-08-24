package snippets

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sshc/internal/platform/windowsacl/acltest"
	"sshc/internal/storage"
)

func newFileStore(t *testing.T) (*Store, *storage.Workspace) {
	t.Helper()
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return NewStore(workspace), workspace
}

func validLibrary() Library {
	moment := time.Date(2026, 8, 24, 8, 0, 0, 0, time.UTC)
	return Library{Snippets: []Snippet{{
		ID: "0123456789abcdef0123456789abcdef", Name: "Check", Command: "uptime", CreatedAt: moment, UpdatedAt: moment,
	}}, Startup: []Startup{{Alias: "bastion", SnippetID: "0123456789abcdef0123456789abcdef"}}}
}

func TestStoreRoundTripsAndAtomicallyReplacesOnePrivateDocument(t *testing.T) {
	store, workspace := newFileStore(t)
	if err := store.Save(validLibrary()); err != nil {
		t.Fatal(err)
	}
	library, err := store.Load()
	if err != nil || len(library.Snippets) != 1 || library.Startup[0].Alias != "bastion" {
		t.Fatalf("Load = %#v, %v", library, err)
	}
	children, err := os.ReadDir(workspace.StateDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 1 || children[0].Name() != filepath.Base(store.Path()) {
		t.Fatalf("state directory = %#v", children)
	}
	assertSnippetDocumentPrivate(t, store.Path())
}

func TestStoreDoesNotOverwriteAnInvalidOrNewerDocument(t *testing.T) {
	for _, fixture := range []struct {
		name string
		body string
		want error
	}{
		{"invalid", `{"schemaVersion":1,"snippets":[{"id":"bad"}]}`, ErrInvalidDocument},
		{"newer", `{"schemaVersion":2,"snippets":[]}`, ErrUnsupportedVersion},
		{"trailing document", `{"schemaVersion":1,"snippets":[]} {}`, ErrInvalidDocument},
		{"unknown field", `{"schemaVersion":1,"snippets":[],"secret":"unexpected"}`, ErrInvalidDocument},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			store, _ := newFileStore(t)
			acltest.WritePrivateFile(t, store.Path(), []byte(fixture.body))
			if _, err := store.Load(); !errors.Is(err, fixture.want) {
				t.Fatalf("Load = %v, want %v", err, fixture.want)
			}
			before, err := os.ReadFile(store.Path())
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != fixture.body {
				t.Fatalf("document changed to %q", before)
			}
		})
	}
}

func TestStoreRejectsDanglingStartupAndSecretDefaults(t *testing.T) {
	store, _ := newFileStore(t)
	library := validLibrary()
	library.Startup[0].SnippetID = "missing"
	if err := store.Save(library); !errors.Is(err, ErrUnknownSnippet) {
		t.Fatalf("Save(dangling) = %v", err)
	}
	secret := "persisted"
	library = validLibrary()
	library.Snippets[0].Command = "echo {{token}}"
	library.Snippets[0].Variables = []Variable{{Name: "token", Type: VariableSecret, Required: true, Default: &secret}}
	library.Startup = nil
	if err := store.Save(library); !errors.Is(err, ErrInvalidVariable) {
		t.Fatalf("Save(secret default) = %v", err)
	}
}
