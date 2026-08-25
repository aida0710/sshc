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
	"sshc/internal/secret"
	"sshc/internal/storage"
	"sshc/internal/validate"
)

const connectionCreateConfig = `# >>> sshc groups (generated). Child groups first: OpenSSH keeps the first value it reads.
# Edit through the UI; lines between these markers are replaced on the next save.
Include connections/home-lab/others/*.conf
Include groups.sshc.conf
# <<< sshc groups

Host existing
	HostName 203.0.113.10
	Port 22
`

const connectionCreatePassphrase = "correct horse battery staple"

type connectionCreateHarness struct {
	service   *Service
	secrets   *secret.Service
	inventory *keys.Inventory
	workspace *storage.Workspace
	manager   *storage.Manager
}

func newConnectionCreateHarness(t *testing.T) connectionCreateHarness {
	t.Helper()
	workspace := newTestWorkspace(t)
	if err := os.WriteFile(filepath.Join(workspace.Root(), "config"), []byte(connectionCreateConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace.Root(), "id_create"), []byte(relocateTestPrivate), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace.Root(), "id_create.pub"), []byte(relocateTestPublic), 0o644); err != nil {
		t.Fatal(err)
	}
	manager := storage.NewManager(workspace, time.Now, rand.Reader)
	service := NewService(workspace, manager)
	secrets := secret.NewService(workspace, manager, time.Now)
	manager.Seal = secrets.SealBackup
	manager.Unseal = secrets.OpenBackup
	if err := secrets.Initialise(connectionCreatePassphrase); err != nil {
		t.Fatal(err)
	}
	return connectionCreateHarness{
		service: service, secrets: secrets, inventory: keyInventory(t, workspace),
		workspace: workspace, manager: manager,
	}
}

func privateKeyID(t *testing.T, inventory *keys.Inventory) string {
	t.Helper()
	for _, item := range inventory.Items {
		if item.Kind == keys.KindPrivateKey && item.RelativePath == "id_create" {
			return item.ID
		}
	}
	t.Fatal("private key fixture was not inventoried")
	return ""
}

func pointerTo[T any](value T) *T { return &value }

func keyCreateRequest(t *testing.T, harness connectionCreateHarness) CreateConnectionRequest {
	t.Helper()
	return CreateConnectionRequest{
		Alias: "lab-node", Group: "home-lab/others", HostName: "2001:db8::1", User: "root",
		Authentication: CreateAuthentication{Kind: CreateAuthenticationIdentityFile, KeyID: privateKeyID(t, harness.inventory)},
	}
}

func TestCreateConnectionWritesACompleteKeyHostIntoAnEmptyNestedGroup(t *testing.T) {
	harness := newConnectionCreateHarness(t)
	request := keyCreateRequest(t, harness)

	result, err := harness.service.CreateConnection(harness.secrets, harness.inventory, request)
	if err != nil {
		t.Fatalf("CreateConnection = %v", err)
	}
	wantPath := "connections/home-lab/others/lab-node.conf"
	want := "Host lab-node\n\tHostName 2001:db8::1\n\tUser root\n\tPort 22\n\tIdentityFile ~/.ssh/id_create\n"
	if got := readFile(t, harness.workspace, wantPath); got != want {
		t.Errorf("created config =\n%s\nwant\n%s", got, want)
	}
	if result.Identity != (HostIdentity{Path: wantPath, Alias: "lab-node"}) {
		t.Errorf("identity = %#v", result.Identity)
	}
	if result.TransactionID == "" || result.Preview.Operation != "connection.create" {
		t.Errorf("result = %#v", result)
	}
	if len(result.Preview.Diffs) != 1 || result.Preview.Diffs[0].Path != wantPath {
		t.Errorf("preview = %#v", result.Preview)
	}
	history, err := harness.manager.History()
	if err != nil {
		t.Fatal(err)
	}
	if len(history) == 0 || history[0].Operation != "connection.create" {
		t.Fatalf("latest transaction = %#v", history)
	}
	for _, written := range history[0].Paths {
		if filepath.Clean(written) == filepath.Join(harness.workspace.Root(), filepath.FromSlash(secret.WorkspacePath)) {
			t.Error("key authentication rewrote the password vault")
		}
	}
	detail, err := harness.service.HostDetail(wantPath, "lab-node")
	if err != nil {
		t.Fatalf("HostDetail of created host = %v", err)
	}
	if detail.Form.Entry.Identity != result.Identity {
		t.Errorf("selected detail identity = %#v", detail.Form.Entry.Identity)
	}
}

func TestCreateConnectionWithoutAGroupAppendsToTheEntryAndOmitsBlankUser(t *testing.T) {
	harness := newConnectionCreateHarness(t)
	request := keyCreateRequest(t, harness)
	request.Alias = "ungrouped"
	request.Group = ""
	request.HostName = "ungrouped.example"
	request.User = ""
	request.Port = pointerTo(2222)

	result, err := harness.service.CreateConnection(harness.secrets, harness.inventory, request)
	if err != nil {
		t.Fatal(err)
	}
	created := readFile(t, harness.workspace, "config")
	if !strings.HasSuffix(created,
		"\nHost ungrouped\n\tHostName ungrouped.example\n\tPort 2222\n\tIdentityFile ~/.ssh/id_create\n") {
		t.Errorf("entry config does not end in the complete host block:\n%s", created)
	}
	if strings.Contains(created[strings.LastIndex(created, "Host ungrouped"):], "\tUser ") {
		t.Error("blank User was written instead of omitted")
	}
	if result.Identity != (HostIdentity{Path: "config", Alias: "ungrouped"}) {
		t.Errorf("identity = %#v", result.Identity)
	}
}

func TestCreateConnectionCommitsEveryPasswordModeWithTheConfig(t *testing.T) {
	tests := []struct {
		name       string
		auth       CreateAuthentication
		prepare    func(*testing.T, *secret.Service)
		wantSecret string
		wantName   string
	}{
		{
			name:       "dedicated",
			auth:       CreateAuthentication{Kind: CreateAuthenticationDedicatedPassword, Password: "connection-only"},
			wantSecret: "connection-only",
		},
		{
			name: "saved password",
			auth: CreateAuthentication{Kind: CreateAuthenticationSavedPassword, Credential: "office"},
			prepare: func(t *testing.T, service *secret.Service) {
				t.Helper()
				if err := service.SetCredential(secret.KindPassword, "office", "shared-secret"); err != nil {
					t.Fatal(err)
				}
			},
			wantSecret: "shared-secret", wantName: "office",
		},
		{
			name: "new shared password",
			auth: CreateAuthentication{
				Kind: CreateAuthenticationNewSharedPassword, Credential: "lab-shared", Password: "new-shared-secret",
			},
			wantSecret: "new-shared-secret", wantName: "lab-shared",
		},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newConnectionCreateHarness(t)
			if test.prepare != nil {
				test.prepare(t, harness.secrets)
			}
			alias := "password-node-" + string(rune('1'+index))
			request := CreateConnectionRequest{
				Alias: alias, Group: "home-lab/others", HostName: "password.example", Port: pointerTo(22),
				Authentication: test.auth,
			}
			result, err := harness.service.CreateConnection(harness.secrets, harness.inventory, request)
			if err != nil {
				t.Fatalf("CreateConnection = %v", err)
			}
			if got := passwordForCurrentTarget(t, harness.service, harness.secrets, alias); got != test.wantSecret {
				t.Errorf("PasswordFor = %q, want %q", got, test.wantSecret)
			}
			listed, err := harness.secrets.Credentials()
			if err != nil {
				t.Fatal(err)
			}
			if test.wantName == "" {
				if _, listedAsReusable := listed[secret.KindPassword][alias]; listedAsReusable {
					t.Error("dedicated password appeared as a reusable credential")
				}
			} else if uses := listed[secret.KindPassword][test.wantName]; !slices.Equal(uses, []string{alias}) {
				t.Errorf("credential uses = %#v", uses)
			}
			history, err := harness.manager.History()
			if err != nil {
				t.Fatal(err)
			}
			configPath := filepath.Join(harness.workspace.Root(), filepath.FromSlash(result.Identity.Path))
			vaultPath := filepath.Join(harness.workspace.Root(), filepath.FromSlash(secret.WorkspacePath))
			if len(history) == 0 || history[0].Operation != "connection.create" ||
				!slices.Contains(history[0].Paths, configPath) || !slices.Contains(history[0].Paths, vaultPath) {
				t.Fatalf("latest transaction = %#v, want config and vault together", history)
			}
			if strings.Contains(readFile(t, harness.workspace, result.Identity.Path), test.wantSecret) {
				t.Error("the SSH config contains the plaintext password")
			}
			preview := result.Preview.Diffs
			if len(preview) != 1 || strings.Contains(preview[0].Path, "sshc/secrets") {
				t.Errorf("preview exposes a vault change: %#v", preview)
			}
		})
	}
}

func TestCreateConnectionBindsASavedCredentialToTheCreatedDestination(t *testing.T) {
	harness := newConnectionCreateHarness(t)
	if err := harness.secrets.SetCredential(secret.KindPassword, "office", "shared-secret"); err != nil {
		t.Fatal(err)
	}
	vaultPath := filepath.Join(harness.workspace.Root(), filepath.FromSlash(secret.WorkspacePath))
	vaultBefore, err := os.ReadFile(vaultPath)
	if err != nil {
		t.Fatal(err)
	}
	result, err := harness.service.CreateConnection(harness.secrets, harness.inventory, CreateConnectionRequest{
		Alias: "restored", HostName: "restored.example", User: "deploy",
		Authentication: CreateAuthentication{Kind: CreateAuthenticationSavedPassword, Credential: "office"},
	})
	if err != nil {
		t.Fatalf("CreateConnection with saved credential = %v", err)
	}
	if result.Identity != (HostIdentity{Path: "config", Alias: "restored"}) {
		t.Fatalf("created identity = %#v", result.Identity)
	}
	if got := readFile(t, harness.workspace, "config"); !strings.Contains(got, "Host restored\n") {
		t.Fatalf("connection was not created:\n%s", got)
	}
	vaultAfter, err := os.ReadFile(vaultPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(vaultAfter, vaultBefore) {
		t.Fatal("the saved credential was not bound to the created destination")
	}
	binding, err := harness.service.PasswordBinding("restored")
	if err != nil {
		t.Fatal(err)
	}
	if got := harness.secrets.BoundPasswordFor("restored", binding); got != "shared-secret" {
		t.Fatalf("bound password = %q, want shared-secret", got)
	}
}

func TestCreateConnectionRejectsInvalidOrConflictingInputsWithoutWriting(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*testing.T, connectionCreateHarness, *CreateConnectionRequest)
		wantErr error
	}{
		{"missing alias", func(_ *testing.T, _ connectionCreateHarness, r *CreateConnectionRequest) { r.Alias = "" }, ErrInvalidAlias},
		{"invalid alias", func(_ *testing.T, _ connectionCreateHarness, r *CreateConnectionRequest) { r.Alias = "bad alias" }, ErrInvalidAlias},
		{"duplicate alias", func(_ *testing.T, _ connectionCreateHarness, r *CreateConnectionRequest) { r.Alias = "existing" }, ErrAliasAlreadyDeclared},
		{"missing host name", func(_ *testing.T, _ connectionCreateHarness, r *CreateConnectionRequest) { r.HostName = "" }, validate.ErrUnsafeHostname},
		{"unsafe user", func(_ *testing.T, _ connectionCreateHarness, r *CreateConnectionRequest) { r.User = "bad user" }, ErrInvalidConnectionUser},
		{"zero port", func(_ *testing.T, _ connectionCreateHarness, r *CreateConnectionRequest) { r.Port = pointerTo(0) }, validate.ErrUnsafePort},
		{"unknown group", func(_ *testing.T, _ connectionCreateHarness, r *CreateConnectionRequest) {
			r.Group = "home-lab/missing"
		}, ErrGroupNotDeclared},
		{"unknown key", func(_ *testing.T, _ connectionCreateHarness, r *CreateConnectionRequest) {
			r.Authentication.KeyID = "missing"
		}, ErrInvalidIdentityFile},
		{"public key", func(_ *testing.T, h connectionCreateHarness, r *CreateConnectionRequest) {
			for _, item := range h.inventory.Items {
				if item.Kind == keys.KindPublicKey {
					r.Authentication.KeyID = item.ID
					return
				}
			}
		}, ErrInvalidIdentityFile},
		{"unknown authentication", func(_ *testing.T, _ connectionCreateHarness, r *CreateConnectionRequest) {
			r.Authentication.Kind = "agent_forwarding"
		}, ErrUnknownCreateAuthentication},
		{"existing destination", func(t *testing.T, h connectionCreateHarness, _ *CreateConnectionRequest) {
			writeGroupFile(t, h.workspace, "home-lab/others", "lab-node.conf", "Host somebody-else\n")
		}, ErrConnectionDestinationExists},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newConnectionCreateHarness(t)
			request := keyCreateRequest(t, harness)
			test.mutate(t, harness, &request)
			beforeConfig := readFile(t, harness.workspace, "config")
			beforeVault, err := os.ReadFile(filepath.Join(harness.workspace.Root(), filepath.FromSlash(secret.WorkspacePath)))
			if err != nil {
				t.Fatal(err)
			}

			_, err = harness.service.CreateConnection(harness.secrets, harness.inventory, request)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("CreateConnection = %v, want %v", err, test.wantErr)
			}
			if got := readFile(t, harness.workspace, "config"); got != beforeConfig {
				t.Error("rejected create changed the entry config")
			}
			afterVault, readErr := os.ReadFile(filepath.Join(harness.workspace.Root(), filepath.FromSlash(secret.WorkspacePath)))
			if readErr != nil {
				t.Fatal(readErr)
			}
			if !bytes.Equal(afterVault, beforeVault) {
				t.Error("rejected create changed the vault")
			}
		})
	}
}

func TestCreateConnectionCommitFailureLeavesConfigAndVaultUnchanged(t *testing.T) {
	harness := newConnectionCreateHarness(t)
	failingManager := storage.NewManager(harness.workspace, time.Now, bytes.NewReader(nil))
	failingService := NewService(harness.workspace, failingManager)
	failingManager.Seal = harness.secrets.SealBackup
	failingManager.Unseal = harness.secrets.OpenBackup
	request := CreateConnectionRequest{
		Alias: "atomic-node", Group: "home-lab/others", HostName: "atomic.example",
		Authentication: CreateAuthentication{Kind: CreateAuthenticationDedicatedPassword, Password: "must-not-survive"},
	}
	beforeConfig := readFile(t, harness.workspace, "config")
	beforeVault, err := os.ReadFile(filepath.Join(harness.workspace.Root(), filepath.FromSlash(secret.WorkspacePath)))
	if err != nil {
		t.Fatal(err)
	}

	_, err = failingService.CreateConnection(harness.secrets, harness.inventory, request)
	if err == nil {
		t.Fatal("CreateConnection succeeded with an exhausted transaction ID source")
	}
	if got := readFile(t, harness.workspace, "config"); got != beforeConfig {
		t.Error("failed commit changed config")
	}
	afterVault, readErr := os.ReadFile(filepath.Join(harness.workspace.Root(), filepath.FromSlash(secret.WorkspacePath)))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(afterVault, beforeVault) {
		t.Error("failed commit changed the vault")
	}
	if got := passwordForCurrentTarget(t, harness.service, harness.secrets, "atomic-node"); got != "" {
		t.Errorf("failed commit published %q in memory", got)
	}
	if _, statErr := os.Stat(filepath.Join(harness.workspace.Root(), "connections", "home-lab", "others", "atomic-node.conf")); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("failed commit left a destination file: %v", statErr)
	}
}
