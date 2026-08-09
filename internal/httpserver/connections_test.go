package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/application"
	"sshc/internal/keys"
	"sshc/internal/secret"
	"sshc/internal/session"
	"sshc/internal/storage"
)

const connectionHTTPConfig = `# >>> sshc groups (generated). Child groups first: OpenSSH keeps the first value it reads.
# Edit through the UI; lines between these markers are replaced on the next save.
Include connections/home-lab/others/*.conf
Include groups.sshc.conf
# <<< sshc groups

Host existing
	HostName 203.0.113.10
	Port 22
`

const connectionHTTPKeyID = "0123456789abcdef0123456789abcdef"

type connectionHTTPHarness struct {
	*testHarness
	passwords *secret.Service
}

func newConnectionHTTPHarness(t *testing.T, initialise bool) *connectionHTTPHarness {
	t.Helper()
	home := t.TempDir()
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.EnsureDirectory(workspace.Root()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace.Root(), "config"), []byte(connectionHTTPConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace.Root(), "id_http"), []byte("private key fixture"), 0o600); err != nil {
		t.Fatal(err)
	}

	manager := storage.NewManager(workspace, time.Now, bytes.NewReader(bytes.Repeat([]byte{0x91}, 8192)))
	service := application.NewService(workspace, manager)
	passwords := secret.NewService(workspace, manager, time.Now)
	manager.Seal = passwords.SealBackup
	manager.Unseal = passwords.OpenBackup
	if initialise {
		if err := passwords.Initialise("correct horse battery staple"); err != nil {
			t.Fatal(err)
		}
	}

	sessions, bootstrap, err := session.NewManager(bytes.NewReader(bytes.Repeat([]byte{0xa2}, 96)))
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := sessions.Bootstrap(bootstrap)
	if err != nil {
		t.Fatal(err)
	}
	engine := echo.New()
	engine.Use((Security{
		ExpectedHost: "127.0.0.1:43123", ExpectedOrigin: "http://127.0.0.1:43123",
		Sessions: sessions, Unlocked: alwaysUnlocked,
	}).Middleware)
	keys := &stubKeyService{inventory: &keys.Inventory{Items: []keys.Item{{
		ID: connectionHTTPKeyID, RelativePath: "id_http", Kind: keys.KindPrivateKey,
	}}}}
	registerConnectionRoutes(engine, ConnectionHandlers{Service: service, Secrets: passwords, Keys: keys})

	return &connectionHTTPHarness{
		testHarness: &testHarness{
			echo: engine, cookie: &http.Cookie{Name: SessionCookie, Value: credentials.SessionID},
			csrf: credentials.CSRFToken, root: workspace.Root(), service: service,
		},
		passwords: passwords,
	}
}

func dedicatedConnectionBody(alias, password string) map[string]any {
	return map[string]any{
		"alias": alias, "group": "home-lab/others", "hostName": "node.example", "user": "ops",
		"authentication": map[string]any{"kind": "dedicated_password", "password": password},
	}
}

func TestCreateConnectionEndpointCommitsAndReturnsOnlySafeData(t *testing.T) {
	harness := newConnectionHTTPHarness(t, true)
	const password = "connection-only-secret"
	response := harness.call(t, http.MethodPost, "/api/v1/connections", dedicatedConnectionBody("new-node", password), true, true)
	if response.Code != http.StatusCreated {
		t.Fatalf("create = %d, body %s", response.Code, response.Body.String())
	}
	var result api.CreateConnectionResponse
	decoder := json.NewDecoder(bytes.NewReader(response.Body.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("response contract = %v", err)
	}
	if result.TransactionId == "" || result.Identity != (api.HostIdentity{
		Path: "connections/home-lab/others/new-node.conf", Alias: "new-node",
	}) {
		t.Fatalf("result = %#v", result)
	}
	if got := harness.passwords.PasswordFor("new-node"); got != password {
		t.Fatalf("stored password = %q", got)
	}
	body := response.Body.String()
	if strings.Contains(body, password) || strings.Contains(body, secret.WorkspacePath) {
		t.Fatal("response exposed plaintext or the encrypted vault change")
	}
	created, err := os.ReadFile(filepath.Join(harness.root, "connections", "home-lab", "others", "new-node.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(created), "\tPort 22\n") {
		t.Fatalf("blank port was not normalised:\n%s", created)
	}
}

func TestCreateConnectionEndpointRequiresSessionAndCSRF(t *testing.T) {
	harness := newConnectionHTTPHarness(t, true)
	body := dedicatedConnectionBody("protected", "secret")
	if got := harness.call(t, http.MethodPost, "/api/v1/connections", body, false, false).Code; got != http.StatusUnauthorized {
		t.Fatalf("without session = %d", got)
	}
	if got := harness.call(t, http.MethodPost, "/api/v1/connections", body, true, false).Code; got != http.StatusForbidden {
		t.Fatalf("without CSRF = %d", got)
	}
}

func TestCreateConnectionEndpointMapsValidationAndConflicts(t *testing.T) {
	tests := []struct {
		name       string
		body       map[string]any
		prepare    func(*testing.T, *connectionHTTPHarness)
		wantStatus int
		wantCode   string
	}{
		{name: "missing alias", body: dedicatedConnectionBody("", "secret"), wantStatus: 400, wantCode: "invalid_request"},
		{name: "duplicate alias", body: dedicatedConnectionBody("existing", "secret"), wantStatus: 409, wantCode: "alias_already_declared"},
		{name: "unknown group", body: func() map[string]any {
			body := dedicatedConnectionBody("unknown-group", "secret")
			body["group"] = "missing"
			return body
		}(), wantStatus: 422, wantCode: "group_not_declared"},
		{name: "unknown key", body: map[string]any{
			"alias": "unknown-key", "hostName": "node.example",
			"authentication": map[string]any{"kind": "identity_file", "keyId": "ffffffffffffffffffffffffffffffff"},
		}, wantStatus: 422, wantCode: "identity_file_invalid"},
		{name: "existing destination", body: dedicatedConnectionBody("occupied", "secret"), prepare: func(t *testing.T, h *connectionHTTPHarness) {
			t.Helper()
			directory := filepath.Join(h.root, "connections", "home-lab", "others")
			if err := os.MkdirAll(directory, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, "occupied.conf"), []byte("Host somebody\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, wantStatus: 409, wantCode: "connection_destination_exists"},
		{name: "unknown authentication", body: map[string]any{
			"alias": "unknown-auth", "hostName": "node.example",
			"authentication": map[string]any{"kind": "agent_forwarding"},
		}, wantStatus: 400, wantCode: "invalid_request"},
		{name: "field from another authentication branch", body: map[string]any{
			"alias": "mixed-auth", "hostName": "node.example",
			"authentication": map[string]any{"kind": "dedicated_password", "password": "secret", "keyId": connectionHTTPKeyID},
		}, wantStatus: 400, wantCode: "invalid_request"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newConnectionHTTPHarness(t, true)
			if test.prepare != nil {
				test.prepare(t, harness)
			}
			response := harness.call(t, http.MethodPost, "/api/v1/connections", test.body, true, true)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d, body %s", response.Code, test.wantStatus, response.Body.String())
			}
			var payload problemPayload
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Code != test.wantCode {
				t.Fatalf("code = %q, want %q", payload.Code, test.wantCode)
			}
		})
	}
}

func TestCreateConnectionEndpointReportsMissingAndLockedVaults(t *testing.T) {
	missing := newConnectionHTTPHarness(t, false)
	response := missing.call(t, http.MethodPost, "/api/v1/connections", dedicatedConnectionBody("missing-vault", "secret"), true, true)
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "vault_missing") {
		t.Fatalf("missing vault = %d %s", response.Code, response.Body.String())
	}

	locked := newConnectionHTTPHarness(t, true)
	locked.passwords.Lock()
	response = locked.call(t, http.MethodPost, "/api/v1/connections", dedicatedConnectionBody("locked-vault", "secret"), true, true)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "vault_locked") {
		t.Fatalf("locked vault = %d %s", response.Code, response.Body.String())
	}
}
