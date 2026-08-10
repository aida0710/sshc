package application

import (
	"bytes"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"sshc/internal/keys"
	"sshc/internal/platform"
	"sshc/internal/secret"
	"sshc/internal/storage"
)

const connectionUpdatePassphrase = "connection update test passphrase"

type connectionUpdateHarness struct {
	service    *Service
	secrets    *secret.Service
	keyService *keys.Service
	inventory  *keys.Inventory
	workspace  *storage.Workspace
	manager    *storage.Manager
}

func newConnectionUpdateHarness(t *testing.T, contents string) connectionUpdateHarness {
	t.Helper()
	workspace := newTestWorkspace(t)
	if err := os.WriteFile(filepath.Join(workspace.Root(), "config"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	writeKeyPair(t, workspace, "id_update")
	manager := storage.NewManager(workspace, time.Now, rand.Reader)
	service := NewService(workspace, manager)
	secrets := secret.NewService(workspace, manager, time.Now)
	manager.Seal = secrets.SealBackup
	manager.Unseal = secrets.OpenBackup
	if err := secrets.Initialise(connectionUpdatePassphrase); err != nil {
		t.Fatal(err)
	}
	keyService := keys.NewService(keys.ServiceOptions{
		Workspace: workspace, Transactions: manager, Resolver: storage.NewResolver(workspace),
	})
	service.SetKeyPassphraseVerifier(keyService)
	return connectionUpdateHarness{
		service: service, secrets: secrets, keyService: keyService, inventory: keyInventory(t, workspace),
		workspace: workspace, manager: manager,
	}
}

func updatePrivateKeyID(t *testing.T, inventory *keys.Inventory) string {
	t.Helper()
	for _, item := range inventory.Items {
		if item.Kind == keys.KindPrivateKey && item.RelativePath == "id_update" {
			return item.ID
		}
	}
	t.Fatal("update private key fixture was not inventoried")
	return ""
}

func unchangedPassword() UpdateConnectionPassword {
	return UpdateConnectionPassword{Kind: UpdatePasswordUnchanged}
}

func unchangedKeyPassphrase() UpdateConnectionKeyPassphrase {
	return UpdateConnectionKeyPassphrase{Kind: UpdateKeyPassphraseUnchanged}
}

func encryptUpdateKey(t *testing.T, harness *connectionUpdateHarness, passphrase string) {
	t.Helper()
	if _, err := harness.keyService.ChangePassphrase(keys.PassphraseChange{
		KeyID: updatePrivateKeyID(t, harness.inventory), New: []byte(passphrase),
	}); err != nil {
		t.Fatalf("encrypt update key: %v", err)
	}
	harness.inventory = keyInventory(t, harness.workspace)
}

func TestUpdateConnectionAddsCommonFieldsToASparseBlock(t *testing.T) {
	const before = "Host edge\n\tServerAliveInterval 30\n\nHost *\n\tUser inherited\n"
	harness := newConnectionUpdateHarness(t, before)
	request := UpdateConnectionRequest{
		Identity: HostIdentity{Path: "config", Alias: "edge"},
		Base:     before,
		HostName: &ConnectionStringChange{Action: ConnectionChangeSet, Value: "198.51.100.8"},
		User:     &ConnectionStringChange{Action: ConnectionChangeSet, Value: "deploy"},
		Port:     &ConnectionPortChange{Action: ConnectionChangeSet, Value: 2222},
		IdentityFile: &ConnectionIdentityFileChange{
			Action: ConnectionChangeSet, KeyID: updatePrivateKeyID(t, harness.inventory),
		},
		Password: unchangedPassword(),
	}

	result, err := harness.service.UpdateConnection(harness.secrets, harness.inventory, request)
	if err != nil {
		t.Fatalf("UpdateConnection = %v", err)
	}
	want := "Host edge\n\tServerAliveInterval 30\n\tHostName 198.51.100.8\n\tUser deploy\n\tPort 2222\n\tIdentityFile ~/.ssh/id_update\n\nHost *\n\tUser inherited\n"
	if got := readFile(t, harness.workspace, "config"); got != want {
		t.Errorf("updated config =\n%s\nwant\n%s", got, want)
	}
	if result.TransactionID == "" || result.Preview.Operation != "connection.update" {
		t.Errorf("result = %#v", result)
	}
	if len(result.Written) != 1 || result.Written[0] != "config" {
		t.Errorf("written = %#v", result.Written)
	}
	if len(result.Preview.Diffs) != 1 || result.Preview.Diffs[0].Path != "config" {
		t.Errorf("preview = %#v", result.Preview)
	}
}

func TestUpdateConnectionReturnsDirectFieldsToInheritance(t *testing.T) {
	const before = "Host edge\r\n\tHostName direct.example\r\n\tUser deploy\r\n\tPort 2222\r\n\tIdentityFile ~/.ssh/id_update\r\n\tServerAliveInterval 30\r\n"
	harness := newConnectionUpdateHarness(t, before)
	request := UpdateConnectionRequest{
		Identity:     HostIdentity{Path: "config", Alias: "edge"},
		Base:         before,
		HostName:     &ConnectionStringChange{Action: ConnectionChangeInherit},
		User:         &ConnectionStringChange{Action: ConnectionChangeInherit},
		Port:         &ConnectionPortChange{Action: ConnectionChangeInherit},
		IdentityFile: &ConnectionIdentityFileChange{Action: ConnectionChangeInherit},
		Password:     unchangedPassword(),
	}

	if _, err := harness.service.UpdateConnection(harness.secrets, harness.inventory, request); err != nil {
		t.Fatalf("UpdateConnection = %v", err)
	}
	want := "Host edge\r\n\tServerAliveInterval 30\r\n"
	if got := readFile(t, harness.workspace, "config"); got != want {
		t.Errorf("updated config = %q, want %q", got, want)
	}
}

func TestUpdateConnectionRefusesDuplicateDirectFields(t *testing.T) {
	const before = "Host edge\n\tUser first\n\tUser second\n"
	harness := newConnectionUpdateHarness(t, before)
	_, err := harness.service.UpdateConnection(harness.secrets, harness.inventory, UpdateConnectionRequest{
		Identity: HostIdentity{Path: "config", Alias: "edge"}, Base: before,
		User:     &ConnectionStringChange{Action: ConnectionChangeSet, Value: "deploy"},
		Password: unchangedPassword(),
	})
	if !errors.Is(err, ErrComplexConnectionField) {
		t.Fatalf("UpdateConnection = %v, want ErrComplexConnectionField", err)
	}
	if got := readFile(t, harness.workspace, "config"); got != before {
		t.Errorf("duplicate rejection changed config: %q", got)
	}
}

func TestUpdateConnectionRejectsInvalidOrEmptyChanges(t *testing.T) {
	const before = "Host edge\n\tHostName edge.example\n"
	tests := []struct {
		name    string
		mutate  func(*testing.T, connectionUpdateHarness, *UpdateConnectionRequest)
		wantErr error
	}{
		{"nothing changed", func(_ *testing.T, _ connectionUpdateHarness, _ *UpdateConnectionRequest) {}, ErrNoConnectionUpdate},
		{"unsafe host", func(_ *testing.T, _ connectionUpdateHarness, r *UpdateConnectionRequest) {
			r.HostName = &ConnectionStringChange{Action: ConnectionChangeSet, Value: "bad host"}
		}, platform.ErrUnsafeHostname},
		{"unsafe user", func(_ *testing.T, _ connectionUpdateHarness, r *UpdateConnectionRequest) {
			r.User = &ConnectionStringChange{Action: ConnectionChangeSet, Value: "bad user"}
		}, ErrInvalidConnectionUser},
		{"unsafe port", func(_ *testing.T, _ connectionUpdateHarness, r *UpdateConnectionRequest) {
			r.Port = &ConnectionPortChange{Action: ConnectionChangeSet, Value: 0}
		}, platform.ErrUnsafePort},
		{"unknown key", func(_ *testing.T, _ connectionUpdateHarness, r *UpdateConnectionRequest) {
			r.IdentityFile = &ConnectionIdentityFileChange{Action: ConnectionChangeSet, KeyID: strings.Repeat("f", 32)}
		}, ErrInvalidIdentityFile},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newConnectionUpdateHarness(t, before)
			request := UpdateConnectionRequest{
				Identity: HostIdentity{Path: "config", Alias: "edge"}, Base: before,
				Password: unchangedPassword(),
			}
			test.mutate(t, harness, &request)
			_, err := harness.service.UpdateConnection(harness.secrets, harness.inventory, request)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("UpdateConnection = %v, want %v", err, test.wantErr)
			}
			if got := readFile(t, harness.workspace, "config"); got != before {
				t.Errorf("rejected update changed config: %q", got)
			}
		})
	}
}

func TestUpdateConnectionRejectsDirectSetToTheExistingValues(t *testing.T) {
	const before = "Host edge\n\tHostName edge.example\n\tUser deploy\n\tPort 22\n"
	harness := newConnectionUpdateHarness(t, before)
	_, err := harness.service.UpdateConnection(harness.secrets, harness.inventory, UpdateConnectionRequest{
		Identity: HostIdentity{Path: "config", Alias: "edge"}, Base: before,
		HostName: &ConnectionStringChange{Action: ConnectionChangeSet, Value: "edge.example"},
		User:     &ConnectionStringChange{Action: ConnectionChangeSet, Value: "deploy"},
		Port:     &ConnectionPortChange{Action: ConnectionChangeSet, Value: 22},
		Password: unchangedPassword(),
	})
	if !errors.Is(err, ErrNoConnectionUpdate) {
		t.Fatalf("UpdateConnection = %v, want ErrNoConnectionUpdate", err)
	}
}

func TestUpdateConnectionRejectsAFileOutsideTheResolvedGraph(t *testing.T) {
	const before = "Host edge\n\tHostName edge.example\n"
	harness := newConnectionUpdateHarness(t, "Host root\n")
	outside := filepath.Join(harness.workspace.Root(), "outside.conf")
	if err := os.WriteFile(outside, []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := harness.service.UpdateConnection(harness.secrets, harness.inventory, UpdateConnectionRequest{
		Identity: HostIdentity{Path: "outside.conf", Alias: "edge"}, Base: before,
		Port:     &ConnectionPortChange{Action: ConnectionChangeSet, Value: 2222},
		Password: unchangedPassword(),
	})
	if !errors.Is(err, ErrHostNotFound) {
		t.Fatalf("UpdateConnection outside graph = %v, want ErrHostNotFound", err)
	}
	if got := readFile(t, harness.workspace, "outside.conf"); got != before {
		t.Fatalf("outside graph file changed: %q", got)
	}
}

type failRenameOnceFileSystem struct {
	storage.FileSystem
	path string
	err  error
}

func (f *failRenameOnceFileSystem) Rename(oldPath, newPath string) error {
	if f.err != nil && filepath.Clean(newPath) == filepath.Clean(f.path) {
		err := f.err
		f.err = nil
		return err
	}
	return f.FileSystem.Rename(oldPath, newPath)
}

func TestUpdateConnectionRollsBackWhenTheSecondFileCommitFails(t *testing.T) {
	const before = "Host edge\n\tHostName edge.example\n\tPort 22\n"
	fileSystem := &failRenameOnceFileSystem{FileSystem: storage.OSFileSystem{}}
	workspace, err := storage.NewWorkspace(fileSystem, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.EnsureDirectory(workspace.Root()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace.Root(), "config"), []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}
	writeKeyPair(t, workspace, "id_update")
	manager := storage.NewManager(workspace, time.Now, rand.Reader)
	service := NewService(workspace, manager)
	secrets := secret.NewService(workspace, manager, time.Now)
	manager.Seal = secrets.SealBackup
	manager.Unseal = secrets.OpenBackup
	if err := secrets.Initialise(connectionUpdatePassphrase); err != nil {
		t.Fatal(err)
	}
	keyService := keys.NewService(keys.ServiceOptions{
		Workspace: workspace, Transactions: manager, Resolver: storage.NewResolver(workspace),
	})
	service.SetKeyPassphraseVerifier(keyService)
	inventory := keyInventory(t, workspace)
	if _, err := keyService.ChangePassphrase(keys.PassphraseChange{
		KeyID: updatePrivateKeyID(t, inventory), New: []byte("correct key phrase"),
	}); err != nil {
		t.Fatal(err)
	}
	inventory = keyInventory(t, workspace)
	vaultPath := filepath.Join(workspace.Root(), filepath.FromSlash(secret.WorkspacePath))
	vaultBefore, err := os.ReadFile(vaultPath)
	if err != nil {
		t.Fatal(err)
	}
	fileSystem.path = vaultPath
	fileSystem.err = errors.New("injected second rename failure")

	_, err = service.UpdateConnection(secrets, inventory, UpdateConnectionRequest{
		Identity: HostIdentity{Path: "config", Alias: "edge"}, Base: before,
		Port: &ConnectionPortChange{Action: ConnectionChangeSet, Value: 2222},
		IdentityFile: &ConnectionIdentityFileChange{
			Action: ConnectionChangeSet, KeyID: updatePrivateKeyID(t, inventory),
		},
		Password: UpdateConnectionPassword{Kind: UpdatePasswordDedicated, Password: "must-not-publish"},
		KeyPassphrase: UpdateConnectionKeyPassphrase{
			Kind: UpdateKeyPassphraseSetDedicated, KeyID: updatePrivateKeyID(t, inventory),
			Passphrase: "correct key phrase",
		},
	})
	if err == nil {
		t.Fatal("UpdateConnection unexpectedly succeeded")
	}
	if got := readFile(t, workspace, "config"); got != before {
		t.Fatalf("config survived failed rollback as %q", got)
	}
	vaultAfter, readErr := os.ReadFile(vaultPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(vaultAfter, vaultBefore) {
		t.Fatal("sealed vault changed after failed transaction")
	}
	if got := secrets.PasswordFor("edge"); got != "" {
		t.Fatalf("memory vault published failed password: %q", got)
	}
	if _, ok := secrets.KeyPassphraseFor("id_update"); ok {
		t.Fatal("memory vault published failed key passphrase")
	}
	pending, pendingErr := manager.Pending()
	if pendingErr != nil {
		t.Fatal(pendingErr)
	}
	if len(pending) != 0 {
		t.Fatalf("rollback left pending transactions: %#v", pending)
	}
}

func TestUpdateConnectionReportsAStaleBaseAsAConflict(t *testing.T) {
	const disk = "Host edge\n\tHostName disk.example\n"
	harness := newConnectionUpdateHarness(t, disk)
	_, err := harness.service.UpdateConnection(harness.secrets, harness.inventory, UpdateConnectionRequest{
		Identity: HostIdentity{Path: "config", Alias: "edge"},
		Base:     "Host edge\n\tHostName stale.example\n",
		Port:     &ConnectionPortChange{Action: ConnectionChangeSet, Value: 2222},
		Password: unchangedPassword(),
	})
	var conflict *ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("UpdateConnection = %v, want ConflictError", err)
	}
	if conflict.Report.Path != "config" {
		t.Errorf("conflict = %#v", conflict.Report)
	}
	if got := readFile(t, harness.workspace, "config"); got != disk {
		t.Errorf("conflict changed config: %q", got)
	}
}

func TestUpdateConnectionCommitsEveryPasswordModeWithTheConfig(t *testing.T) {
	const before = "Host edge\n\tHostName edge.example\n\tPort 22\n"
	tests := []struct {
		name       string
		password   UpdateConnectionPassword
		prepare    func(*testing.T, *secret.Service)
		wantSecret string
		wantName   string
	}{
		{
			name: "dedicated", password: UpdateConnectionPassword{
				Kind: UpdatePasswordDedicated, Password: "connection-only",
			}, wantSecret: "connection-only",
		},
		{
			name: "saved", password: UpdateConnectionPassword{
				Kind: UpdatePasswordSaved, Credential: "office",
			},
			prepare: func(t *testing.T, service *secret.Service) {
				t.Helper()
				if err := service.SetCredential(secret.KindPassword, "office", "shared-secret"); err != nil {
					t.Fatal(err)
				}
			},
			wantSecret: "shared-secret", wantName: "office",
		},
		{
			name: "new shared", password: UpdateConnectionPassword{
				Kind: UpdatePasswordNewShared, Credential: "lab", Password: "lab-secret",
			}, wantSecret: "lab-secret", wantName: "lab",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newConnectionUpdateHarness(t, before)
			if test.prepare != nil {
				test.prepare(t, harness.secrets)
			}
			result, err := harness.service.UpdateConnection(harness.secrets, harness.inventory, UpdateConnectionRequest{
				Identity: HostIdentity{Path: "config", Alias: "edge"}, Base: before,
				Port:     &ConnectionPortChange{Action: ConnectionChangeSet, Value: 2222},
				Password: test.password,
			})
			if err != nil {
				t.Fatalf("UpdateConnection = %v", err)
			}
			if got := harness.secrets.PasswordFor("edge"); got != test.wantSecret {
				t.Errorf("PasswordFor(edge) = %q, want %q", got, test.wantSecret)
			}
			listed, err := harness.secrets.Credentials()
			if err != nil {
				t.Fatal(err)
			}
			if test.wantName == "" {
				if _, reusable := listed[secret.KindPassword]["edge"]; reusable {
					t.Error("dedicated password appeared in reusable credentials")
				}
			} else if uses := listed[secret.KindPassword][test.wantName]; !slices.Equal(uses, []string{"edge"}) {
				t.Errorf("credential uses = %#v", uses)
			}
			if !slices.Equal(result.Written, []string{"config"}) {
				t.Errorf("public written paths = %#v", result.Written)
			}
			if len(result.Preview.Diffs) != 1 || result.Preview.Diffs[0].Path != "config" {
				t.Errorf("preview = %#v", result.Preview)
			}
			if strings.Contains(readFile(t, harness.workspace, "config"), test.wantSecret) {
				t.Error("SSH config contains the plaintext password")
			}
			history, err := harness.manager.History()
			if err != nil {
				t.Fatal(err)
			}
			configPath := filepath.Join(harness.workspace.Root(), "config")
			vaultPath := filepath.Join(harness.workspace.Root(), filepath.FromSlash(secret.WorkspacePath))
			if len(history) == 0 || history[0].Operation != "connection.update" ||
				!slices.Contains(history[0].Paths, configPath) || !slices.Contains(history[0].Paths, vaultPath) {
				t.Errorf("transaction history = %#v", history)
			}
		})
	}
}

func TestUpdateConnectionCanChangeOnlyTheStoredPassword(t *testing.T) {
	const before = "Host edge\n\tHostName edge.example\n"
	harness := newConnectionUpdateHarness(t, before)
	result, err := harness.service.UpdateConnection(harness.secrets, harness.inventory, UpdateConnectionRequest{
		Identity: HostIdentity{Path: "config", Alias: "edge"}, Base: before,
		Password: UpdateConnectionPassword{Kind: UpdatePasswordDedicated, Password: "connection-only"},
	})
	if err != nil {
		t.Fatalf("UpdateConnection = %v", err)
	}
	if got := readFile(t, harness.workspace, "config"); got != before {
		t.Errorf("password-only update changed config: %q", got)
	}
	if got := harness.secrets.PasswordFor("edge"); got != "connection-only" {
		t.Errorf("PasswordFor(edge) = %q", got)
	}
	if len(result.Written) != 0 || len(result.Preview.Diffs) != 0 {
		t.Errorf("password-only public result exposes vault path: %#v", result)
	}
}

func TestUpdateConnectionSkipsASemanticallyUnchangedPasswordAssignment(t *testing.T) {
	const before = "Host edge\n\tHostName edge.example\n\tPort 22\n"
	harness := newConnectionUpdateHarness(t, before)
	if err := harness.secrets.SetCredential(secret.KindPassword, "office", "shared"); err != nil {
		t.Fatal(err)
	}
	if err := harness.secrets.AssignCredential(secret.KindPassword, "edge", "office"); err != nil {
		t.Fatal(err)
	}
	vaultPath := filepath.Join(harness.workspace.Root(), filepath.FromSlash(secret.WorkspacePath))
	vaultBefore, err := os.ReadFile(vaultPath)
	if err != nil {
		t.Fatal(err)
	}
	request := UpdateConnectionRequest{
		Identity: HostIdentity{Path: "config", Alias: "edge"}, Base: before,
		Password: UpdateConnectionPassword{Kind: UpdatePasswordSaved, Credential: "office"},
	}
	if _, err := harness.service.UpdateConnection(harness.secrets, harness.inventory, request); !errors.Is(err, ErrNoConnectionUpdate) {
		t.Fatalf("same assignment = %v, want ErrNoConnectionUpdate", err)
	}
	request.Port = &ConnectionPortChange{Action: ConnectionChangeSet, Value: 2222}
	if _, err := harness.service.UpdateConnection(harness.secrets, harness.inventory, request); err != nil {
		t.Fatalf("config change with same assignment = %v", err)
	}
	if got := readFile(t, harness.workspace, "config"); !strings.Contains(got, "Port 2222") {
		t.Fatalf("config-only part was not saved: %q", got)
	}
	vaultAfter, err := os.ReadFile(vaultPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(vaultAfter, vaultBefore) {
		t.Fatal("same assignment resealed the vault")
	}
}

func TestUpdateConnectionNewSharedCollisionChangesNeitherConfigNorVault(t *testing.T) {
	const before = "Host edge\n\tHostName edge.example\n\tPort 22\n"
	harness := newConnectionUpdateHarness(t, before)
	if err := harness.secrets.SetCredential(secret.KindPassword, "office", "must-survive"); err != nil {
		t.Fatal(err)
	}
	vaultPath := filepath.Join(harness.workspace.Root(), filepath.FromSlash(secret.WorkspacePath))
	vaultBefore, err := os.ReadFile(vaultPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = harness.service.UpdateConnection(harness.secrets, harness.inventory, UpdateConnectionRequest{
		Identity: HostIdentity{Path: "config", Alias: "edge"}, Base: before,
		Port:     &ConnectionPortChange{Action: ConnectionChangeSet, Value: 2222},
		Password: UpdateConnectionPassword{Kind: UpdatePasswordNewShared, Credential: "office", Password: "replacement"},
	})
	if !errors.Is(err, secret.ErrCredentialAlreadyExists) {
		t.Fatalf("new shared collision = %v, want ErrCredentialAlreadyExists", err)
	}
	if got := readFile(t, harness.workspace, "config"); got != before {
		t.Fatalf("collision changed config: %q", got)
	}
	vaultAfter, err := os.ReadFile(vaultPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(vaultAfter, vaultBefore) {
		t.Fatal("collision changed sealed vault")
	}
}

func TestUpdateConnectionRemovesDedicatedOrUnassignsReusablePassword(t *testing.T) {
	const before = "Host edge\n\tHostName edge.example\n"
	tests := []struct {
		name    string
		prepare func(*testing.T, *secret.Service)
		verify  func(*testing.T, *secret.Service)
	}{
		{
			name: "dedicated",
			prepare: func(t *testing.T, service *secret.Service) {
				t.Helper()
				if err := service.Set("edge", "dedicated"); err != nil {
					t.Fatal(err)
				}
			},
			verify: func(t *testing.T, service *secret.Service) {
				t.Helper()
				if got := service.PasswordFor("edge"); got != "" {
					t.Errorf("dedicated password survived: %q", got)
				}
			},
		},
		{
			name: "reusable",
			prepare: func(t *testing.T, service *secret.Service) {
				t.Helper()
				if err := service.SetCredential(secret.KindPassword, "office", "shared"); err != nil {
					t.Fatal(err)
				}
				if err := service.AssignCredential(secret.KindPassword, "edge", "office"); err != nil {
					t.Fatal(err)
				}
			},
			verify: func(t *testing.T, service *secret.Service) {
				t.Helper()
				listed, err := service.Credentials()
				if err != nil {
					t.Fatal(err)
				}
				uses, exists := listed[secret.KindPassword]["office"]
				if !exists || len(uses) != 0 || service.PasswordFor("edge") != "" {
					t.Errorf("reusable password after removal = %#v, exists %t", uses, exists)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newConnectionUpdateHarness(t, before)
			test.prepare(t, harness.secrets)
			result, err := harness.service.UpdateConnection(harness.secrets, harness.inventory, UpdateConnectionRequest{
				Identity: HostIdentity{Path: "config", Alias: "edge"}, Base: before,
				Password: UpdateConnectionPassword{Kind: UpdatePasswordRemove},
			})
			if err != nil {
				t.Fatalf("UpdateConnection(remove) = %v", err)
			}
			test.verify(t, harness.secrets)
			if len(result.Written) != 0 || len(result.Preview.Diffs) != 0 {
				t.Errorf("remove result exposes vault: %#v", result)
			}
		})
	}
}

func TestUpdateConnectionConfigOnlyDoesNotDispatchAPasswordMutation(t *testing.T) {
	const before = "Host edge\n\tHostName edge.example\n"
	harness := newConnectionUpdateHarness(t, before)
	if _, err := harness.service.UpdateConnection(nil, harness.inventory, UpdateConnectionRequest{
		Identity: HostIdentity{Path: "config", Alias: "edge"}, Base: before,
		Port:     &ConnectionPortChange{Action: ConnectionChangeSet, Value: 2222},
		Password: unchangedPassword(),
	}); err != nil {
		t.Fatalf("config-only update consulted password mutation service = %v", err)
	}

	changed := readFile(t, harness.workspace, "config")
	harness.secrets.Lock()
	_, err := harness.service.UpdateConnection(harness.secrets, harness.inventory, UpdateConnectionRequest{
		Identity: HostIdentity{Path: "config", Alias: "edge"}, Base: changed,
		Password: UpdateConnectionPassword{Kind: UpdatePasswordDedicated, Password: "blocked"},
	})
	if !errors.Is(err, secret.ErrLocked) {
		t.Fatalf("password update with locked vault = %v, want ErrLocked", err)
	}
}

func TestUpdateConnectionRejectsAnIneligiblePassword(t *testing.T) {
	const before = "Host edge\n\tHostName edge.example\n\tPasswordAuthentication no\n"
	harness := newConnectionUpdateHarness(t, before)
	_, err := harness.service.UpdateConnection(harness.secrets, harness.inventory, UpdateConnectionRequest{
		Identity: HostIdentity{Path: "config", Alias: "edge"}, Base: before,
		Password: UpdateConnectionPassword{Kind: UpdatePasswordDedicated, Password: "blocked"},
	})
	if !errors.Is(err, ErrPasswordIneligible) {
		t.Fatalf("UpdateConnection = %v, want ErrPasswordIneligible", err)
	}
	if got := harness.secrets.PasswordFor("edge"); got != "" {
		t.Errorf("ineligible password was stored: %q", got)
	}
}

func TestUpdateConnectionPasswordConflictPublishesNothing(t *testing.T) {
	const disk = "Host edge\n\tHostName disk.example\n\tIdentityFile ~/.ssh/id_update\n"
	harness := newConnectionUpdateHarness(t, disk)
	encryptUpdateKey(t, &harness, "correct key phrase")
	beforeVault, err := os.ReadFile(filepath.Join(harness.workspace.Root(), filepath.FromSlash(secret.WorkspacePath)))
	if err != nil {
		t.Fatal(err)
	}
	_, err = harness.service.UpdateConnection(harness.secrets, harness.inventory, UpdateConnectionRequest{
		Identity: HostIdentity{Path: "config", Alias: "edge"},
		Base:     "Host edge\n\tHostName stale.example\n",
		Port:     &ConnectionPortChange{Action: ConnectionChangeSet, Value: 2222},
		Password: UpdateConnectionPassword{Kind: UpdatePasswordDedicated, Password: "must-not-survive"},
		KeyPassphrase: UpdateConnectionKeyPassphrase{
			Kind: UpdateKeyPassphraseSetDedicated, KeyID: updatePrivateKeyID(t, harness.inventory),
			Passphrase: "correct key phrase",
		},
	})
	var conflict *ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("UpdateConnection = %v, want ConflictError", err)
	}
	afterVault, readErr := os.ReadFile(filepath.Join(harness.workspace.Root(), filepath.FromSlash(secret.WorkspacePath)))
	if readErr != nil {
		t.Fatal(readErr)
	}
	_, hasKeyPassphrase := harness.secrets.KeyPassphraseFor("id_update")
	if !bytes.Equal(afterVault, beforeVault) || harness.secrets.PasswordFor("edge") != "" || hasKeyPassphrase {
		t.Error("conflicted update changed vault disk or memory")
	}
}

func TestUpdateConnectionCommitFailureLeavesConfigAndVaultUnchanged(t *testing.T) {
	const before = "Host edge\n\tHostName edge.example\n\tPort 22\n"
	harness := newConnectionUpdateHarness(t, before)
	encryptUpdateKey(t, &harness, "correct key phrase")
	failingManager := storage.NewManager(harness.workspace, time.Now, bytes.NewReader(nil))
	failingService := NewService(harness.workspace, failingManager)
	failingService.SetKeyPassphraseVerifier(harness.keyService)
	failingManager.Seal = harness.secrets.SealBackup
	failingManager.Unseal = harness.secrets.OpenBackup
	beforeVault, err := os.ReadFile(filepath.Join(harness.workspace.Root(), filepath.FromSlash(secret.WorkspacePath)))
	if err != nil {
		t.Fatal(err)
	}

	_, err = failingService.UpdateConnection(harness.secrets, harness.inventory, UpdateConnectionRequest{
		Identity: HostIdentity{Path: "config", Alias: "edge"}, Base: before,
		Port: &ConnectionPortChange{Action: ConnectionChangeSet, Value: 2222},
		IdentityFile: &ConnectionIdentityFileChange{
			Action: ConnectionChangeSet, KeyID: updatePrivateKeyID(t, harness.inventory),
		},
		Password: UpdateConnectionPassword{Kind: UpdatePasswordDedicated, Password: "must-not-survive"},
		KeyPassphrase: UpdateConnectionKeyPassphrase{
			Kind: UpdateKeyPassphraseSetDedicated, KeyID: updatePrivateKeyID(t, harness.inventory),
			Passphrase: "correct key phrase",
		},
	})
	if err == nil {
		t.Fatal("UpdateConnection succeeded with an exhausted transaction ID source")
	}
	if got := readFile(t, harness.workspace, "config"); got != before {
		t.Errorf("failed update changed config: %q", got)
	}
	afterVault, readErr := os.ReadFile(filepath.Join(harness.workspace.Root(), filepath.FromSlash(secret.WorkspacePath)))
	if readErr != nil {
		t.Fatal(readErr)
	}
	_, hasKeyPassphrase := harness.secrets.KeyPassphraseFor("id_update")
	if !bytes.Equal(afterVault, beforeVault) || harness.secrets.PasswordFor("edge") != "" || hasKeyPassphrase {
		t.Error("failed update changed vault disk or memory")
	}
}

func TestUpdateConnectionSavesPassphraseForTheSelectedEncryptedKeyOnly(t *testing.T) {
	const before = "Host edge\n\tIdentityFile ~/.ssh/id_update\n"
	harness := newConnectionUpdateHarness(t, before)
	encryptUpdateKey(t, &harness, "correct key phrase")
	result, err := harness.service.UpdateConnection(harness.secrets, harness.inventory, UpdateConnectionRequest{
		Identity: HostIdentity{Path: "config", Alias: "edge"}, Base: before,
		Password: unchangedPassword(),
		KeyPassphrase: UpdateConnectionKeyPassphrase{
			Kind: UpdateKeyPassphraseSetDedicated, KeyID: updatePrivateKeyID(t, harness.inventory),
			Passphrase: "correct key phrase",
		},
	})
	if err != nil {
		t.Fatalf("UpdateConnection = %v", err)
	}
	if got, ok := harness.secrets.KeyPassphraseFor("id_update"); !ok || got != "correct key phrase" {
		t.Fatalf("stored key passphrase = %q, %v", got, ok)
	}
	if got := readFile(t, harness.workspace, "config"); got != before {
		t.Fatalf("passphrase-only save changed config: %q", got)
	}
	if len(result.Written) != 0 || len(result.Preview.Diffs) != 0 {
		t.Fatalf("passphrase-only result exposed vault: %#v", result)
	}
}

func TestUpdateConnectionSavesIdentityPasswordAndKeyPassphraseInOneTransaction(t *testing.T) {
	const before = "Host edge\n\tHostName edge.example\n"
	harness := newConnectionUpdateHarness(t, before)
	encryptUpdateKey(t, &harness, "correct key phrase")
	result, err := harness.service.UpdateConnection(harness.secrets, harness.inventory, UpdateConnectionRequest{
		Identity: HostIdentity{Path: "config", Alias: "edge"}, Base: before,
		IdentityFile: &ConnectionIdentityFileChange{
			Action: ConnectionChangeSet, KeyID: updatePrivateKeyID(t, harness.inventory),
		},
		Password: UpdateConnectionPassword{Kind: UpdatePasswordDedicated, Password: "account-secret"},
		KeyPassphrase: UpdateConnectionKeyPassphrase{
			Kind: UpdateKeyPassphraseSetDedicated, KeyID: updatePrivateKeyID(t, harness.inventory),
			Passphrase: "correct key phrase",
		},
	})
	if err != nil {
		t.Fatalf("UpdateConnection = %v", err)
	}
	if got := readFile(t, harness.workspace, "config"); !strings.Contains(got, "IdentityFile ~/.ssh/id_update") {
		t.Fatalf("config = %q", got)
	}
	if harness.secrets.PasswordFor("edge") != "account-secret" {
		t.Fatal("account password was not committed")
	}
	if got, ok := harness.secrets.KeyPassphraseFor("id_update"); !ok || got != "correct key phrase" {
		t.Fatalf("key passphrase = %q, %v", got, ok)
	}
	if !slices.Equal(result.Written, []string{"config"}) {
		t.Fatalf("written = %#v", result.Written)
	}
	history, err := harness.manager.History()
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(harness.workspace.Root(), "config")
	vaultPath := filepath.Join(harness.workspace.Root(), filepath.FromSlash(secret.WorkspacePath))
	if len(history) == 0 || history[0].Operation != "connection.update" ||
		!slices.Contains(history[0].Paths, configPath) || !slices.Contains(history[0].Paths, vaultPath) {
		t.Fatalf("combined transaction history = %#v", history)
	}
}

func TestUpdateConnectionPassphraseDetachesOnlyTheSelectedKeyFromSharedCredential(t *testing.T) {
	const before = "Host edge\n\tIdentityFile ~/.ssh/id_update\n"
	harness := newConnectionUpdateHarness(t, before)
	encryptUpdateKey(t, &harness, "correct key phrase")
	if _, err := harness.keyService.Generate(keys.GenerateRequest{
		Algorithm: keys.AlgorithmEd25519, FileName: "id_other", Comment: "other",
		Passphrase: []byte("shared old phrase"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := harness.secrets.SetCredential(secret.KindKeyPassphrase, "team", "shared old phrase"); err != nil {
		t.Fatal(err)
	}
	for _, subject := range []string{"id_update", "id_other"} {
		if err := harness.secrets.AssignCredential(secret.KindKeyPassphrase, subject, "team"); err != nil {
			t.Fatal(err)
		}
	}
	_, err := harness.service.UpdateConnection(harness.secrets, harness.inventory, UpdateConnectionRequest{
		Identity: HostIdentity{Path: "config", Alias: "edge"}, Base: before,
		Password: unchangedPassword(),
		KeyPassphrase: UpdateConnectionKeyPassphrase{
			Kind: UpdateKeyPassphraseSetDedicated, KeyID: updatePrivateKeyID(t, harness.inventory),
			Passphrase: "correct key phrase",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := harness.secrets.KeyPassphraseFor("id_update"); got != "correct key phrase" {
		t.Fatalf("selected key = %q", got)
	}
	if got, _ := harness.secrets.KeyPassphraseFor("id_other"); got != "shared old phrase" {
		t.Fatalf("other shared key changed: %q", got)
	}
	listed, err := harness.secrets.Credentials()
	if err != nil {
		t.Fatal(err)
	}
	if uses := listed[secret.KindKeyPassphrase]["team"]; !slices.Equal(uses, []string{"id_other"}) {
		t.Fatalf("shared uses = %#v", uses)
	}
}

func TestUpdateConnectionPassphraseRejectsWrongUnencryptedUnrelatedCustomAndComplexKeys(t *testing.T) {
	tests := []struct {
		name    string
		before  string
		prepare func(*testing.T, *connectionUpdateHarness) (string, string)
		want    error
	}{
		{
			name: "wrong passphrase", before: "Host edge\n\tIdentityFile ~/.ssh/id_update\n",
			prepare: func(t *testing.T, h *connectionUpdateHarness) (string, string) {
				encryptUpdateKey(t, h, "correct key phrase")
				return updatePrivateKeyID(t, h.inventory), "wrong key phrase"
			}, want: keys.ErrWrongPassphrase,
		},
		{
			name: "unencrypted", before: "Host edge\n\tIdentityFile ~/.ssh/id_update\n",
			prepare: func(t *testing.T, h *connectionUpdateHarness) (string, string) {
				return updatePrivateKeyID(t, h.inventory), "anything"
			}, want: keys.ErrKeyNotEncrypted,
		},
		{
			name: "unrelated", before: "Host edge\n\tIdentityFile ~/.ssh/id_update\n",
			prepare: func(t *testing.T, h *connectionUpdateHarness) (string, string) {
				if _, err := h.keyService.Generate(keys.GenerateRequest{
					Algorithm: keys.AlgorithmEd25519, FileName: "id_other", Comment: "other",
					Passphrase: []byte("other phrase"),
				}); err != nil {
					t.Fatal(err)
				}
				h.inventory = keyInventory(t, h.workspace)
				return keys.ItemID("id_other"), "other phrase"
			}, want: ErrInvalidIdentityFile,
		},
		{
			name: "custom path", before: "Host edge\n\tIdentityFile ~/.ssh/custom-key\n",
			prepare: func(t *testing.T, h *connectionUpdateHarness) (string, string) {
				encryptUpdateKey(t, h, "correct key phrase")
				return updatePrivateKeyID(t, h.inventory), "correct key phrase"
			}, want: ErrInvalidIdentityFile,
		},
		{
			name: "multiple direct keys", before: "Host edge\n\tIdentityFile ~/.ssh/id_update\n\tIdentityFile ~/.ssh/other\n",
			prepare: func(t *testing.T, h *connectionUpdateHarness) (string, string) {
				encryptUpdateKey(t, h, "correct key phrase")
				return updatePrivateKeyID(t, h.inventory), "correct key phrase"
			}, want: ErrInvalidIdentityFile,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newConnectionUpdateHarness(t, test.before)
			keyID, passphrase := test.prepare(t, &harness)
			beforeVault, err := os.ReadFile(filepath.Join(harness.workspace.Root(), filepath.FromSlash(secret.WorkspacePath)))
			if err != nil {
				t.Fatal(err)
			}
			_, err = harness.service.UpdateConnection(harness.secrets, harness.inventory, UpdateConnectionRequest{
				Identity: HostIdentity{Path: "config", Alias: "edge"}, Base: test.before,
				Password: unchangedPassword(),
				KeyPassphrase: UpdateConnectionKeyPassphrase{
					Kind: UpdateKeyPassphraseSetDedicated, KeyID: keyID, Passphrase: passphrase,
				},
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("UpdateConnection = %v, want %v", err, test.want)
			}
			if got := readFile(t, harness.workspace, "config"); got != test.before {
				t.Fatalf("rejection changed config: %q", got)
			}
			afterVault, readErr := os.ReadFile(filepath.Join(harness.workspace.Root(), filepath.FromSlash(secret.WorkspacePath)))
			if readErr != nil {
				t.Fatal(readErr)
			}
			if !bytes.Equal(afterVault, beforeVault) {
				t.Fatal("rejection changed sealed vault")
			}
		})
	}
}

type changingKeyVerifier struct {
	service   *keys.Service
	workspace *storage.Workspace
}

func (v changingKeyVerifier) VerifyPassphrase(keyID string, passphrase []byte) (keys.PassphraseVerification, error) {
	verification, err := v.service.VerifyPassphrase(keyID, passphrase)
	if err != nil {
		return keys.PassphraseVerification{}, err
	}
	keyPath := filepath.Join(v.workspace.Root(), filepath.FromSlash(verification.RelativePath))
	contents, err := os.ReadFile(keyPath)
	if err != nil {
		return keys.PassphraseVerification{}, err
	}
	if err := os.WriteFile(keyPath, append(contents, '\n'), 0o600); err != nil {
		return keys.PassphraseVerification{}, err
	}
	return verification, nil
}

func (v changingKeyVerifier) RevalidatePassphrase(verification keys.PassphraseVerification) error {
	return v.service.RevalidatePassphrase(verification)
}

func TestUpdateConnectionPassphraseRejectsKeyBytesChangedBeforeCommit(t *testing.T) {
	const before = "Host edge\n\tIdentityFile ~/.ssh/id_update\n"
	harness := newConnectionUpdateHarness(t, before)
	encryptUpdateKey(t, &harness, "correct key phrase")
	harness.service.SetKeyPassphraseVerifier(changingKeyVerifier{
		service: harness.keyService, workspace: harness.workspace,
	})
	beforeVault, err := os.ReadFile(filepath.Join(harness.workspace.Root(), filepath.FromSlash(secret.WorkspacePath)))
	if err != nil {
		t.Fatal(err)
	}
	_, err = harness.service.UpdateConnection(harness.secrets, harness.inventory, UpdateConnectionRequest{
		Identity: HostIdentity{Path: "config", Alias: "edge"}, Base: before,
		Password: unchangedPassword(),
		KeyPassphrase: UpdateConnectionKeyPassphrase{
			Kind: UpdateKeyPassphraseSetDedicated, KeyID: updatePrivateKeyID(t, harness.inventory),
			Passphrase: "correct key phrase",
		},
	})
	if !errors.Is(err, keys.ErrKeyChanged) {
		t.Fatalf("UpdateConnection = %v, want ErrKeyChanged", err)
	}
	afterVault, readErr := os.ReadFile(filepath.Join(harness.workspace.Root(), filepath.FromSlash(secret.WorkspacePath)))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(afterVault, beforeVault) {
		t.Fatal("changed key rejection changed vault")
	}
}
