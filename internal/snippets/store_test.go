package snippets

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	protection := Protection{
		Seal: func(plain []byte) ([]byte, error) {
			sealed := append([]byte("sealed\x00"), plain...)
			for index := len("sealed\x00"); index < len(sealed); index++ {
				sealed[index] ^= 0xaa
			}
			return sealed, nil
		},
		Open: func(sealed []byte) ([]byte, error) {
			if !bytes.HasPrefix(sealed, []byte("sealed\x00")) {
				return nil, ErrNotEncrypted
			}
			plain := append([]byte(nil), sealed[len("sealed\x00"):]...)
			for index := range plain {
				plain[index] ^= 0xaa
			}
			return plain, nil
		},
		WithMutation: func(mutation func() error) error { return mutation() },
	}
	return NewStore(workspace, protection), workspace
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
	sealed, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("uptime")) || bytes.Contains(sealed, []byte("bastion")) {
		t.Fatalf("encrypted document contains snippet plaintext: %q", sealed)
	}
}

func TestStoreReportsOnlySuccessfulChanges(t *testing.T) {
	store, _ := newFileStore(t)
	changed := 0
	store.SetAfterChange(func() { changed++ })

	if err := store.Save(validLibrary()); err != nil {
		t.Fatal(err)
	}
	rejected := validLibrary()
	rejected.Startup[0].SnippetID = "missing"
	if err := store.Save(rejected); !errors.Is(err, ErrUnknownSnippet) {
		t.Fatalf("invalid Save = %v, want ErrUnknownSnippet", err)
	}
	if err := store.Mutate(func(library *Library) error {
		library.Snippets[0].Name = "Changed"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	wantFailure := errors.New("stop mutation")
	if err := store.Mutate(func(*Library) error { return wantFailure }); !errors.Is(err, wantFailure) {
		t.Fatalf("rejected Mutate = %v, want %v", err, wantFailure)
	}
	if changed != 2 {
		t.Fatalf("AfterChange called %d times, want 2 successful writes", changed)
	}
}

func TestStoreRefusesADocumentWhichItsBoundedReaderCannotOpen(t *testing.T) {
	store, _ := newFileStore(t)
	moment := time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)
	library := Library{Snippets: make([]Snippet, 17)}
	for index := range library.Snippets {
		library.Snippets[index] = Snippet{
			ID: fmt.Sprintf("%032x", index+1), Name: fmt.Sprintf("large-%d", index),
			Command: strings.Repeat("x", MaxCommandBytes), CreatedAt: moment, UpdatedAt: moment,
		}
	}
	if err := store.Save(library); !errors.Is(err, storage.ErrFileTooLarge) {
		t.Fatalf("Save = %v, want ErrFileTooLarge", err)
	}
	if _, err := os.Stat(store.Path()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("oversized document was published: %v", err)
	}
}

func TestStoreMigratesAValidLegacyPlaintextDocument(t *testing.T) {
	store, _ := newFileStore(t)
	plain, err := encodeDocument(validLibrary())
	if err != nil {
		t.Fatal(err)
	}
	acltest.WritePrivateFile(t, store.Path(), plain)
	if _, err := store.Load(); err != nil {
		t.Fatal(err)
	}
	sealed, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(sealed, []byte("sealed\x00")) || bytes.Contains(sealed, []byte("uptime")) {
		t.Fatalf("legacy document was not encrypted: %q", sealed)
	}
}

func TestTravelDocumentMigratesAValidLegacyPlaintextDocument(t *testing.T) {
	store, _ := newFileStore(t)
	plain, err := encodeDocument(validLibrary())
	if err != nil {
		t.Fatal(err)
	}
	acltest.WritePrivateFile(t, store.Path(), plain)
	travelled, err := store.TravelDocument()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(travelled, plain) {
		t.Fatalf("TravelDocument = %q, want canonical legacy plaintext", travelled)
	}
	sealed, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(sealed, []byte("sealed\x00")) || bytes.Contains(sealed, []byte("uptime")) {
		t.Fatalf("travel read left the legacy document in plaintext: %q", sealed)
	}
}

func TestExpansionRejectsTheFinalSizeBeforeBuildingTheCommand(t *testing.T) {
	value := strings.Repeat("x", MaxVariableValueBytes)
	command := strings.Repeat("{{value}}", MaxCommandBytes/MaxVariableValueBytes+1)
	_, err := expand(command, []Variable{{Name: "value", Type: VariableString, Required: true}}, map[string]string{"value": value})
	if !errors.Is(err, ErrInvalidSnippet) {
		t.Fatalf("expand oversized result = %v, want ErrInvalidSnippet", err)
	}
}

func TestStoreDoesNotOverwriteAnInvalidOrNewerDocument(t *testing.T) {
	for _, fixture := range []struct {
		name string
		body string
		want error
	}{
		{"invalid", `{"schemaVersion":1,"snippets":[{"id":"bad"}]}`, ErrInvalidDocument},
		{"newer", `{"schemaVersion":3,"snippets":[]}`, ErrUnsupportedVersion},
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

func TestCloneSnippetKeepsRequiredEmptyVariablesAsAnArray(t *testing.T) {
	cloned := cloneSnippet(Snippet{Variables: nil})
	if cloned.Variables == nil {
		t.Fatal("cloneSnippet returned nil variables; the API contract requires an array")
	}
}

// 形式 1 の割り当ては接続先の binding を持たない。割り当てた時の接続先は分からない
// ので結び付けずに読み、接続時には送らない。
func TestAVersionOneStartupIsReadWithoutADestinationBinding(t *testing.T) {
	store, _ := newFileStore(t)
	current, err := encodeDocument(validLibrary())
	if err != nil {
		t.Fatal(err)
	}
	legacy := bytes.Replace(current, []byte(`"schemaVersion": 2`), []byte(`"schemaVersion": 1`), 1)
	if bytes.Equal(legacy, current) {
		t.Fatalf("the fixture is not a version 1 document: %q", current)
	}
	acltest.WritePrivateFile(t, store.Path(), legacy)

	library, err := store.Load()
	if err != nil {
		t.Fatalf("Load = %v", err)
	}
	if len(library.Startup) != 1 || library.Startup[0].Binding != "" {
		t.Fatalf("startup = %#v, want one assignment without a binding", library.Startup)
	}
}

// VPN プロファイルの改名や削除に加わると、rebind が書き換えを返した割り当てだけの
// 結び付けの値を変える。空の値を返された割り当ては停止中になる。
func TestAStartupRebindChangesOnlyTheAssignmentsItIsGivenValuesFor(t *testing.T) {
	const (
		throughLab    = "1111111111111111111111111111111111111111111111111111111111111111"
		throughLab2   = "2222222222222222222222222222222222222222222222222222222222222222"
		throughOffice = "3333333333333333333333333333333333333333333333333333333333333333"
	)
	store, _ := newFileStore(t)
	library := validLibrary()
	snippetID := library.Snippets[0].ID
	library.Startup = []Startup{
		{Alias: "lab", SnippetID: snippetID, Binding: throughLab},
		{Alias: "removed", SnippetID: snippetID, Binding: throughLab},
		{Alias: "office", SnippetID: snippetID, Binding: throughOffice},
	}
	if err := store.Save(library); err != nil {
		t.Fatal(err)
	}
	rebind := func(alias, binding string) (string, bool) {
		switch {
		case binding != throughLab:
			return "", false
		case alias == "lab":
			return throughLab2, true
		default:
			return "", true
		}
	}

	if _, err := store.WithStartupRebind(rebind, func(change *storage.Change) (storage.Result, error) {
		if change == nil {
			t.Fatal("the rebind produced no change")
		}
		acltest.WritePrivateFile(t, change.Path, change.Contents)
		return storage.Result{}, nil
	}); err != nil {
		t.Fatalf("WithStartupRebind = %v", err)
	}

	rebound, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, startup := range rebound.Startup {
		got[startup.Alias] = startup.Binding
	}
	want := map[string]string{"lab": throughLab2, "removed": "", "office": throughOffice}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("bindings = %v, want %v", got, want)
	}
}
