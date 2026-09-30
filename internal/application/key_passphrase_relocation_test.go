package application

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sshc/internal/keys"
	"sshc/internal/secret"
	"sshc/internal/storage"
)

const keyRelocationPassphrase = "correct horse battery staple"

type keyRelocationFixture struct {
	service    *Service
	secrets    *secret.Service
	workspace  *storage.Workspace
	manager    *storage.Manager
	fileSystem *failRenameOnceFileSystem
}

// newKeyRelocationFixture は、グループ work の鍵 keys/work/id_work に名前付き
// パスフレーズを割り当てた workspace を作る。
func newKeyRelocationFixture(t *testing.T) keyRelocationFixture {
	t.Helper()
	fileSystem := &failRenameOnceFileSystem{FileSystem: storage.OSFileSystem{}}
	workspace, err := storage.NewWorkspace(fileSystem, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.EnsureDirectory(filepath.Join(workspace.Root(), "conf.d")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace.Root(), "config"), []byte(serviceMainConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := storage.NewManager(workspace, time.Now, bytes.NewReader(bytes.Repeat([]byte{0x6b}, 8192)))
	service := NewService(workspace, manager)
	secrets := secret.NewService(workspace, manager, time.Now)
	manager.Seal = secrets.SealBackup
	manager.Unseal = secrets.OpenBackup
	if err := secrets.Initialise(keyRelocationPassphrase); err != nil {
		t.Fatal(err)
	}
	service.SetVault(secrets)

	declareGroup(t, service, "work")
	writeGroupFile(t, workspace, "work", "web.conf", "Host web-1\n\tHostName 203.0.113.10\n")
	writeKeyPair(t, workspace, "keys/work/id_work")
	if err := os.WriteFile(filepath.Join(workspace.Root(), "conf.d", "30-keys.conf"),
		[]byte("Host web-1\n\tIdentityFile ~/.ssh/keys/work/id_work\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := secrets.SetCredential(secret.KindKeyPassphrase, "work", "phrase"); err != nil {
		t.Fatal(err)
	}
	if err := secrets.AssignCredential(secret.KindKeyPassphrase, "keys/work/id_work", "work"); err != nil {
		t.Fatal(err)
	}
	return keyRelocationFixture{
		service: service, secrets: secrets, workspace: workspace, manager: manager, fileSystem: fileSystem,
	}
}

// keyPathChanges は、鍵のパスを変える 3 つのユースケースと、そのあとの鍵のパスである。
var keyPathChanges = []struct {
	name   string
	change func(t *testing.T, fixture keyRelocationFixture) error
	moved  string
}{
	{
		name: "group rename",
		change: func(t *testing.T, fixture keyRelocationFixture) error {
			_, err := fixture.service.RenameGroup(keyInventory(t, fixture.workspace), "work", "client-a")
			return err
		},
		moved: "keys/client-a/id_work",
	},
	{
		name: "group delete",
		change: func(t *testing.T, fixture keyRelocationFixture) error {
			_, err := fixture.service.DeleteGroup(keyInventory(t, fixture.workspace), "work", "")
			return err
		},
		moved: "keys/id_work",
	},
	{
		name: "key rename",
		change: func(t *testing.T, fixture keyRelocationFixture) error {
			_, err := fixture.service.RelocateKey(keyInventory(t, fixture.workspace), KeyRelocateRequest{
				KeyID: keys.ItemID("keys/work/id_work"), NewName: stringPointer("id_client"),
			})
			return err
		},
		moved: "keys/work/id_client",
	},
}

func TestChangingAKeyPathCarriesItsPassphraseAssignmentToTheNewPath(t *testing.T) {
	for _, test := range keyPathChanges {
		t.Run(test.name, func(t *testing.T) {
			fixture := newKeyRelocationFixture(t)
			if err := test.change(t, fixture); err != nil {
				t.Fatal(err)
			}
			if value, ok := fixture.secrets.KeyPassphraseFor(test.moved); !ok || value != "phrase" {
				t.Errorf("passphrase at the new path = %q, %v", value, ok)
			}
			if _, ok := fixture.secrets.KeyPassphraseFor("keys/work/id_work"); ok {
				t.Error("the old path still resolves to the passphrase")
			}
		})
	}
}

func TestChangingAKeyPathMovesNothingWhenTheVaultCannotBeWritten(t *testing.T) {
	for _, test := range keyPathChanges {
		t.Run(test.name, func(t *testing.T) {
			fixture := newKeyRelocationFixture(t)
			configBefore := readFile(t, fixture.workspace, "conf.d/30-keys.conf")
			injected := errors.New("injected vault write failure")
			fixture.fileSystem.path = filepath.Join(fixture.workspace.Root(), filepath.FromSlash(secret.WorkspacePath))
			fixture.fileSystem.err = injected

			if err := test.change(t, fixture); !errors.Is(err, injected) {
				t.Fatalf("change with a failing vault write = %v, want the injected failure", err)
			}
			if _, err := os.Lstat(filepath.Join(fixture.workspace.Root(), "keys", "work", "id_work")); err != nil {
				t.Errorf("the key left its old path: %v", err)
			}
			if _, err := os.Lstat(filepath.Join(fixture.workspace.Root(), filepath.FromSlash(test.moved))); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("the key reached its new path: %v", err)
			}
			if got := readFile(t, fixture.workspace, "conf.d/30-keys.conf"); got != configBefore {
				t.Errorf("IdentityFile was rewritten: %q", got)
			}
			if value, ok := fixture.secrets.KeyPassphraseFor("keys/work/id_work"); !ok || value != "phrase" {
				t.Errorf("passphrase at the old path = %q, %v", value, ok)
			}
			pending, err := fixture.manager.Pending()
			if err != nil || len(pending) != 0 {
				t.Errorf("pending transactions = %#v, %v", pending, err)
			}
		})
	}
}

func TestChangingAKeyPathOnALockedVaultIsRefusedBeforeAnythingMoves(t *testing.T) {
	fixture := newKeyRelocationFixture(t)
	fixture.secrets.Lock()

	_, err := fixture.service.RenameGroup(keyInventory(t, fixture.workspace), "work", "client-a")
	if !errors.Is(err, secret.ErrLocked) {
		t.Fatalf("RenameGroup on a locked vault = %v, want ErrLocked", err)
	}
	if _, err := os.Lstat(filepath.Join(fixture.workspace.Root(), "keys", "work", "id_work")); err != nil {
		t.Errorf("the key left its old path: %v", err)
	}
}

func TestChangingAKeyPathWithoutAWiredVaultIsRefusedBeforeAnythingMoves(t *testing.T) {
	for _, test := range keyPathChanges {
		t.Run(test.name, func(t *testing.T) {
			fixture := newKeyRelocationFixture(t)
			fixture.service.SetVault(nil)
			configBefore := readFile(t, fixture.workspace, "conf.d/30-keys.conf")

			if err := test.change(t, fixture); !errors.Is(err, ErrKeyPassphraseVaultMissing) {
				t.Fatalf("change without a vault = %v, want ErrKeyPassphraseVaultMissing", err)
			}
			if _, err := os.Lstat(filepath.Join(fixture.workspace.Root(), "keys", "work", "id_work")); err != nil {
				t.Errorf("the key left its old path: %v", err)
			}
			if got := readFile(t, fixture.workspace, "conf.d/30-keys.conf"); got != configBefore {
				t.Errorf("IdentityFile was rewritten: %q", got)
			}
		})
	}
}
