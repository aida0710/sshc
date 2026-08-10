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
	keys      *stubKeyService
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
	keyStub := &stubKeyService{
		verifyPhrase: "correct key phrase",
		inventory: &keys.Inventory{
			Items: []keys.Item{{
				ID: connectionHTTPKeyID, RelativePath: "id_http", Kind: keys.KindPrivateKey, Encrypted: true,
			}},
		},
	}
	service.SetKeyPassphraseVerifier(keyStub)
	registerConnectionRoutes(engine, ConnectionHandlers{Service: service, Secrets: passwords, Keys: keyStub})
	registerPasswordRoutes(engine, PasswordHandlers{Service: passwords})

	return &connectionHTTPHarness{
		testHarness: &testHarness{
			echo: engine, cookie: &http.Cookie{Name: SessionCookie, Value: credentials.SessionID},
			csrf: credentials.CSRFToken, root: workspace.Root(), service: service,
		},
		passwords: passwords, keys: keyStub,
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

func connectionUpdateBody(password map[string]any) map[string]any {
	return map[string]any{
		"identity":      map[string]any{"path": "config", "alias": "existing"},
		"base":          connectionHTTPConfig,
		"user":          map[string]any{"action": "set", "value": "deploy"},
		"port":          map[string]any{"action": "set", "value": 2222},
		"password":      password,
		"keyPassphrase": map[string]any{"kind": "unchanged"},
	}
}

func TestUpdateConnectionEndpointCommitsAndReturnsOnlySafeData(t *testing.T) {
	harness := newConnectionHTTPHarness(t, true)
	const password = "updated-connection-secret"
	response := harness.call(t, http.MethodPatch, "/api/v1/connections", connectionUpdateBody(map[string]any{
		"kind": "dedicated_password", "password": password,
	}), true, true)
	if response.Code != http.StatusOK {
		t.Fatalf("update = %d, body %s", response.Code, response.Body.String())
	}
	var result api.SaveResult
	decoder := json.NewDecoder(bytes.NewReader(response.Body.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("response contract = %v", err)
	}
	if result.TransactionId == "" || len(result.Written) != 1 || result.Written[0] != "config" {
		t.Fatalf("result = %#v", result)
	}
	if got := harness.passwords.PasswordFor("existing"); got != password {
		t.Fatalf("stored password = %q", got)
	}
	updated, err := os.ReadFile(filepath.Join(harness.root, "config"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "\tUser deploy\n") || !strings.Contains(string(updated), "\tPort 2222\n") {
		t.Fatalf("updated config =\n%s", updated)
	}
	body := response.Body.String()
	if strings.Contains(body, password) || strings.Contains(body, secret.WorkspacePath) {
		t.Fatal("response exposed plaintext or the encrypted vault change")
	}
}

func TestUpdateConnectionEndpointRequiresSessionAndCSRF(t *testing.T) {
	harness := newConnectionHTTPHarness(t, true)
	body := connectionUpdateBody(map[string]any{"kind": "unchanged"})
	if got := harness.call(t, http.MethodPatch, "/api/v1/connections", body, false, false).Code; got != http.StatusUnauthorized {
		t.Fatalf("without session = %d", got)
	}
	if got := harness.call(t, http.MethodPatch, "/api/v1/connections", body, true, false).Code; got != http.StatusForbidden {
		t.Fatalf("without CSRF = %d", got)
	}
}

func TestUpdateConnectionEndpointDecodesEveryPasswordMutation(t *testing.T) {
	tests := []struct {
		name     string
		password map[string]any
		prepare  func(*testing.T, *connectionHTTPHarness)
		want     string
	}{
		{
			name: "saved", password: map[string]any{"kind": "saved_password", "credential": "office"},
			prepare: func(t *testing.T, harness *connectionHTTPHarness) {
				t.Helper()
				if err := harness.passwords.SetCredential(secret.KindPassword, "office", "shared-secret"); err != nil {
					t.Fatal(err)
				}
			}, want: "shared-secret",
		},
		{
			name: "new shared", password: map[string]any{
				"kind": "new_shared_password", "credential": "lab", "password": "lab-secret",
			}, want: "lab-secret",
		},
		{
			name: "remove", password: map[string]any{"kind": "remove"},
			prepare: func(t *testing.T, harness *connectionHTTPHarness) {
				t.Helper()
				if err := harness.passwords.Set("existing", "remove-me"); err != nil {
					t.Fatal(err)
				}
			}, want: "",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newConnectionHTTPHarness(t, true)
			if test.prepare != nil {
				test.prepare(t, harness)
			}
			response := harness.call(t, http.MethodPatch, "/api/v1/connections", connectionUpdateBody(test.password), true, true)
			if response.Code != http.StatusOK {
				t.Fatalf("update = %d, body %s", response.Code, response.Body.String())
			}
			if got := harness.passwords.PasswordFor("existing"); got != test.want {
				t.Errorf("PasswordFor(existing) = %q, want %q", got, test.want)
			}
		})
	}
}

func TestUpdateConnectionEndpointMapsValidationAndConflicts(t *testing.T) {
	tests := []struct {
		name       string
		body       map[string]any
		prepare    func(*testing.T, *connectionHTTPHarness)
		wantStatus int
		wantCode   string
	}{
		{
			name: "no change", body: map[string]any{
				"identity": map[string]any{"path": "config", "alias": "existing"},
				"base":     connectionHTTPConfig, "password": map[string]any{"kind": "unchanged"},
				"keyPassphrase": map[string]any{"kind": "unchanged"},
			}, wantStatus: 400, wantCode: "connection_no_change",
		},
		{
			name: "unknown top field", body: func() map[string]any {
				body := connectionUpdateBody(map[string]any{"kind": "unchanged"})
				body["unexpected"] = true
				return body
			}(), wantStatus: 400, wantCode: "invalid_request",
		},
		{
			name: "unknown setting field", body: func() map[string]any {
				body := connectionUpdateBody(map[string]any{"kind": "unchanged"})
				body["port"] = map[string]any{"action": "set", "value": 2222, "unexpected": true}
				return body
			}(), wantStatus: 400, wantCode: "invalid_request",
		},
		{
			name: "unsafe host name", body: func() map[string]any {
				body := connectionUpdateBody(map[string]any{"kind": "unchanged"})
				body["hostName"] = map[string]any{"action": "set", "value": "bad host"}
				return body
			}(), wantStatus: 400, wantCode: "connection_hostname_invalid",
		},
		{
			name: "unsafe user", body: func() map[string]any {
				body := connectionUpdateBody(map[string]any{"kind": "unchanged"})
				body["user"] = map[string]any{"action": "set", "value": "bad user"}
				return body
			}(), wantStatus: 400, wantCode: "connection_user_invalid",
		},
		{
			name: "unsafe port", body: func() map[string]any {
				body := connectionUpdateBody(map[string]any{"kind": "unchanged"})
				body["port"] = map[string]any{"action": "set", "value": 70000}
				return body
			}(), wantStatus: 400, wantCode: "connection_port_invalid",
		},
		{
			name: "unknown password branch", body: connectionUpdateBody(map[string]any{
				"kind": "agent_forwarding",
			}), wantStatus: 400, wantCode: "invalid_request",
		},
		{
			name: "password branch mixed with key", body: connectionUpdateBody(map[string]any{
				"kind": "dedicated_password", "password": "secret", "keyId": connectionHTTPKeyID,
			}), wantStatus: 400, wantCode: "invalid_request",
		},
		{
			name: "unknown key", body: func() map[string]any {
				body := connectionUpdateBody(map[string]any{"kind": "unchanged"})
				delete(body, "user")
				delete(body, "port")
				body["identityFile"] = map[string]any{"action": "set", "keyId": strings.Repeat("f", 32)}
				return body
			}(), wantStatus: 422, wantCode: "identity_file_invalid",
		},
		{
			name: "complex field", body: func() map[string]any {
				body := connectionUpdateBody(map[string]any{"kind": "unchanged"})
				body["base"] = strings.Replace(connectionHTTPConfig, "\tPort 22\n", "\tPort 22\n\tUser first\n\tUser second\n", 1)
				return body
			}(),
			prepare: func(t *testing.T, harness *connectionHTTPHarness) {
				t.Helper()
				contents := strings.Replace(connectionHTTPConfig, "\tPort 22\n", "\tPort 22\n\tUser first\n\tUser second\n", 1)
				if err := os.WriteFile(filepath.Join(harness.root, "config"), []byte(contents), 0o600); err != nil {
					t.Fatal(err)
				}
			}, wantStatus: 422, wantCode: "connection_field_complex",
		},
		{
			name: "stale base", body: func() map[string]any {
				body := connectionUpdateBody(map[string]any{"kind": "unchanged"})
				body["base"] = strings.Replace(connectionHTTPConfig, "203.0.113.10", "stale.example", 1)
				return body
			}(), wantStatus: 409, wantCode: "config_conflict",
		},
		{
			name: "password ineligible", body: func() map[string]any {
				body := connectionUpdateBody(map[string]any{"kind": "dedicated_password", "password": "secret"})
				body["base"] = strings.Replace(connectionHTTPConfig, "\tPort 22\n", "\tPort 22\n\tPasswordAuthentication no\n", 1)
				return body
			}(),
			prepare: func(t *testing.T, harness *connectionHTTPHarness) {
				t.Helper()
				contents := strings.Replace(connectionHTTPConfig, "\tPort 22\n", "\tPort 22\n\tPasswordAuthentication no\n", 1)
				if err := os.WriteFile(filepath.Join(harness.root, "config"), []byte(contents), 0o600); err != nil {
					t.Fatal(err)
				}
			}, wantStatus: 422, wantCode: "password_ineligible",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newConnectionHTTPHarness(t, true)
			if test.prepare != nil {
				test.prepare(t, harness)
			}
			response := harness.call(t, http.MethodPatch, "/api/v1/connections", test.body, true, true)
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

func TestUpdateConnectionEndpointSavesDedicatedKeyPassphraseWithoutReturningIt(t *testing.T) {
	harness := newConnectionHTTPHarness(t, true)
	const passphrase = "correct key phrase"
	body := connectionUpdateBody(map[string]any{"kind": "unchanged"})
	body["identityFile"] = map[string]any{"action": "set", "keyId": connectionHTTPKeyID}
	body["keyPassphrase"] = map[string]any{
		"kind": "set_dedicated", "keyId": connectionHTTPKeyID, "passphrase": passphrase,
	}
	response := harness.call(t, http.MethodPatch, "/api/v1/connections", body, true, true)
	if response.Code != http.StatusOK {
		t.Fatalf("update = %d, body %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), passphrase) || strings.Contains(response.Body.String(), secret.WorkspacePath) {
		t.Fatal("success response exposed the passphrase or vault path")
	}
	if got, ok := harness.passwords.KeyPassphraseFor("id_http"); !ok || got != passphrase {
		t.Fatalf("stored key passphrase = %q, %v", got, ok)
	}

	status := harness.call(t, http.MethodGet, "/api/v1/passwords", nil, true, true)
	if status.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", status.Code, status.Body.String())
	}
	var payload struct {
		Dedicated []string `json:"dedicatedKeyPassphrases"`
	}
	if err := json.Unmarshal(status.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Dedicated) != 1 || payload.Dedicated[0] != "id_http" {
		t.Fatalf("dedicated key paths = %#v", payload.Dedicated)
	}
	if strings.Contains(status.Body.String(), passphrase) {
		t.Fatal("vault status exposed the key passphrase")
	}
}

func TestUpdateConnectionEndpointRejectsInvalidKeyPassphraseUnionMembers(t *testing.T) {
	tests := []struct {
		name  string
		value any
	}{
		{name: "missing", value: nil},
		{name: "unknown kind", value: map[string]any{"kind": "shared"}},
		{name: "unchanged with secret", value: map[string]any{"kind": "unchanged", "passphrase": "secret"}},
		{name: "set missing key", value: map[string]any{"kind": "set_dedicated", "passphrase": "secret"}},
		{name: "set missing phrase", value: map[string]any{"kind": "set_dedicated", "keyId": connectionHTTPKeyID}},
		{name: "set extra field", value: map[string]any{
			"kind": "set_dedicated", "keyId": connectionHTTPKeyID, "passphrase": "secret", "extra": true,
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newConnectionHTTPHarness(t, true)
			body := connectionUpdateBody(map[string]any{"kind": "unchanged"})
			if test.value == nil {
				delete(body, "keyPassphrase")
			} else {
				body["keyPassphrase"] = test.value
			}
			response := harness.call(t, http.MethodPatch, "/api/v1/connections", body, true, true)
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_request") {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestUpdateConnectionEndpointMapsWrongAndStaleKeyPassphrasesWithoutEchoingThem(t *testing.T) {
	tests := []struct {
		name       string
		prepare    func(*connectionHTTPHarness)
		passphrase string
		wantStatus int
		wantCode   string
	}{
		{
			name: "wrong", passphrase: "wrong secret phrase",
			wantStatus: http.StatusForbidden, wantCode: "wrong_passphrase",
		},
		{
			name: "key changed", passphrase: "correct key phrase",
			prepare:    func(h *connectionHTTPHarness) { h.keys.revalidateErr = keys.ErrKeyChanged },
			wantStatus: http.StatusConflict, wantCode: "external_change",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newConnectionHTTPHarness(t, true)
			if test.prepare != nil {
				test.prepare(harness)
			}
			body := connectionUpdateBody(map[string]any{"kind": "unchanged"})
			body["identityFile"] = map[string]any{"action": "set", "keyId": connectionHTTPKeyID}
			body["keyPassphrase"] = map[string]any{
				"kind": "set_dedicated", "keyId": connectionHTTPKeyID, "passphrase": test.passphrase,
			}
			response := harness.call(t, http.MethodPatch, "/api/v1/connections", body, true, true)
			if response.Code != test.wantStatus || !strings.Contains(response.Body.String(), test.wantCode) {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
			if strings.Contains(response.Body.String(), test.passphrase) {
				t.Fatal("problem response echoed the submitted passphrase")
			}
		})
	}
}
