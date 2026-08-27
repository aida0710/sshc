package httpserver

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/handoff"
	"sshc/internal/remotesync"
	"sshc/internal/secret"
	"sshc/internal/session"
	"sshc/internal/storage"
	"sshc/internal/terminal"
)

const (
	testVaultStatusPath = "/cli/vault/status"
	testVaultCreatePath = "/cli/vault/create"
	testVaultUnlockPath = "/cli/vault/unlock"
	testVaultLockPath   = "/cli/vault/lock"
	testVaultChangePath = "/cli/vault/change-password"
	testCLISecret       = "the secret for this run"
)

func newCLIVaultService(t *testing.T) *secret.Service {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	return secret.NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader), time.Now)
}

func cliHeaders(secret string) map[string]string {
	return map[string]string{handoff.HeaderName: secret}
}

func TestCLIVaultRoutesRequireTheCurrentHandoffSecret(t *testing.T) {
	service := newCLIVaultService(t)
	engine := connectEngine(t, ConnectHandlers{Secret: testCLISecret, Passwords: service})
	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "status", method: http.MethodGet, path: testVaultStatusPath},
		{name: "create", method: http.MethodPost, path: testVaultCreatePath, body: `{"passphrase":"a valid master password"}`},
		{name: "unlock", method: http.MethodPost, path: testVaultUnlockPath, body: `{"passphrase":"a valid master password"}`},
		{name: "lock", method: http.MethodPost, path: testVaultLockPath, body: `{}`},
		{name: "change", method: http.MethodPost, path: testVaultChangePath, body: `{"current":"a valid master password","next":"another valid password"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, headers := range []map[string]string{nil, cliHeaders("not the current secret")} {
				response := send(t, engine, test.method, test.path, test.body, headers)
				if response.Code != http.StatusUnauthorized {
					t.Errorf("status = %d, want 401", response.Code)
				}
			}
		})
	}
}

func TestCLIVaultPostRoutesRejectUnknownAndTrailingJSON(t *testing.T) {
	service := newCLIVaultService(t)
	engine := connectEngine(t, ConnectHandlers{Secret: testCLISecret, Passwords: service})
	tests := []struct {
		name string
		path string
		body string
	}{
		{name: "create unknown", path: testVaultCreatePath, body: `{"passphrase":"a valid master password","unknown":true}`},
		{name: "unlock unknown", path: testVaultUnlockPath, body: `{"passphrase":"a valid master password","unknown":true}`},
		{name: "lock unknown", path: testVaultLockPath, body: `{"unknown":true}`},
		{name: "change unknown", path: testVaultChangePath, body: `{"current":"a valid master password","next":"another valid password","unknown":true}`},
		{name: "create trailing", path: testVaultCreatePath, body: `{"passphrase":"a valid master password"} {}`},
		{name: "unlock trailing", path: testVaultUnlockPath, body: `{"passphrase":"a valid master password"} {}`},
		{name: "lock trailing", path: testVaultLockPath, body: `{} {}`},
		{name: "change trailing", path: testVaultChangePath, body: `{"current":"a valid master password","next":"another valid password"} {}`},
		{name: "create malformed", path: testVaultCreatePath, body: `{"passphrase":`},
		{name: "unlock malformed", path: testVaultUnlockPath, body: `{"passphrase":`},
		{name: "lock malformed", path: testVaultLockPath, body: `{`},
		{name: "change malformed", path: testVaultChangePath, body: `{"current":`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := send(t, engine, http.MethodPost, test.path, test.body, cliHeaders(testCLISecret))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestCLIVaultPostRoutesRejectOversizedJSON(t *testing.T) {
	service := newCLIVaultService(t)
	engine := connectEngine(t, ConnectHandlers{Secret: testCLISecret, Passwords: service})
	for _, path := range []string{testVaultCreatePath, testVaultUnlockPath, testVaultLockPath, testVaultChangePath} {
		t.Run(path, func(t *testing.T) {
			response := send(t, engine, http.MethodPost, path,
				`{"passphrase":"`+strings.Repeat("x", 5<<10)+`"}`, cliHeaders(testCLISecret))
			if response.Code != http.StatusRequestEntityTooLarge {
				t.Fatalf("status = %d, want 413", response.Code)
			}
		})
	}
}

func TestCLIVaultCreateMapsVaultOutcomes(t *testing.T) {
	t.Run("success then existing", func(t *testing.T) {
		service := newCLIVaultService(t)
		engine := connectEngine(t, ConnectHandlers{Secret: testCLISecret, Passwords: service})
		body := `{"passphrase":"` + testPassphrase + `"}`
		if response := send(t, engine, http.MethodPost, testVaultCreatePath, body, cliHeaders(testCLISecret)); response.Code != http.StatusNoContent {
			t.Fatalf("create = %d, want 204: %s", response.Code, response.Body.String())
		}
		if response := send(t, engine, http.MethodPost, testVaultCreatePath, body, cliHeaders(testCLISecret)); response.Code != http.StatusConflict {
			t.Fatalf("second create = %d, want 409", response.Code)
		}
	})

	t.Run("short passphrase", func(t *testing.T) {
		service := newCLIVaultService(t)
		engine := connectEngine(t, ConnectHandlers{Secret: testCLISecret, Passwords: service})
		response := send(t, engine, http.MethodPost, testVaultCreatePath, `{"passphrase":"short"}`, cliHeaders(testCLISecret))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("create = %d, want 400", response.Code)
		}
	})
}

func TestCLIVaultUnlockMapsMissingWrongAndSuccess(t *testing.T) {
	service := newCLIVaultService(t)
	engine := connectEngine(t, ConnectHandlers{Secret: testCLISecret, Passwords: service})
	if response := send(t, engine, http.MethodPost, testVaultUnlockPath, `{"passphrase":"anything at all"}`, cliHeaders(testCLISecret)); response.Code != http.StatusConflict {
		t.Fatalf("missing vault = %d, want 409", response.Code)
	}
	if err := service.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	service.Lock()
	wrong := send(t, engine, http.MethodPost, testVaultUnlockPath, `{"passphrase":"this is the wrong password"}`, cliHeaders(testCLISecret))
	if wrong.Code != http.StatusUnauthorized {
		t.Fatalf("wrong passphrase = %d, want 401", wrong.Code)
	}
	if strings.Contains(wrong.Body.String(), "wrong") || strings.Contains(wrong.Body.String(), "decrypt") {
		t.Fatalf("wrong-passphrase response leaks internals: %s", wrong.Body.String())
	}
	if response := send(t, engine, http.MethodPost, testVaultUnlockPath,
		`{"passphrase":"`+testPassphrase+`"}`, cliHeaders(testCLISecret)); response.Code != http.StatusNoContent {
		t.Fatalf("unlock = %d, want 204: %s", response.Code, response.Body.String())
	}
}

func TestCLIVaultChangeMapsLockedAndWrongCurrent(t *testing.T) {
	service := newCLIVaultService(t)
	if err := service.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	engine := connectEngine(t, ConnectHandlers{Secret: testCLISecret, Passwords: service})
	service.Lock()
	// lock 状態そのものを先に報告する。current を検証できる状態ではないため、
	// 入力が違っていても password oracle にせず 409 へ畳む。
	body := `{"current":"not the current password","next":"another valid password"}`
	if response := send(t, engine, http.MethodPost, testVaultChangePath, body, cliHeaders(testCLISecret)); response.Code != http.StatusConflict {
		t.Fatalf("locked change = %d, want 409", response.Code)
	}
	if err := service.Unlock(testPassphrase); err != nil {
		t.Fatal(err)
	}
	wrong := send(t, engine, http.MethodPost, testVaultChangePath,
		`{"current":"not the current password","next":"another valid password"}`, cliHeaders(testCLISecret))
	if wrong.Code != http.StatusUnauthorized {
		t.Fatalf("wrong current = %d, want 401", wrong.Code)
	}
}

func TestCLIVaultStatusDescribesMissingLockedAndUnlocked(t *testing.T) {
	service := newCLIVaultService(t)
	engine := connectEngine(t, ConnectHandlers{
		Secret: testCLISecret, Passwords: service, Sessions: func() int { return 2 },
		Owner: handoff.OwnerEngine, Version: "v-status-test", ProtocolVersion: handoff.ProtocolVersion,
	})
	assertStatus := func(vault, unlocked bool) {
		t.Helper()
		response := send(t, engine, http.MethodGet, testVaultStatusPath, "", cliHeaders(testCLISecret))
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", response.Code, response.Body.String())
		}
		var answer CLIStatus
		if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
			t.Fatal(err)
		}
		if answer.Vault != vault || answer.Unlocked != unlocked || answer.Sessions != 2 {
			t.Fatalf("answer = %+v, want vault=%v unlocked=%v sessions=2", answer, vault, unlocked)
		}
		if answer.Owner != handoff.OwnerEngine || answer.Version != "v-status-test" || answer.ProtocolVersion != handoff.ProtocolVersion {
			t.Fatalf("identity = owner %q, version %q, protocol %d", answer.Owner, answer.Version, answer.ProtocolVersion)
		}
	}
	assertStatus(false, false)
	if err := service.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	service.Lock()
	assertStatus(true, false)
	if err := service.Unlock(testPassphrase); err != nil {
		t.Fatal(err)
	}
	assertStatus(true, true)
}

func TestCLIVaultStatusDoesNotResetVaultInactivity(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	service := secret.NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader), func() time.Time { return now })
	if err := service.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	engine := connectEngine(t, ConnectHandlers{Secret: testCLISecret, Passwords: service})
	now = now.Add(secret.IdleTimeout - time.Second)
	if response := send(t, engine, http.MethodGet, testVaultStatusPath, "", cliHeaders(testCLISecret)); response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	now = now.Add(2 * time.Second)
	if service.Unlocked() {
		t.Fatal("status read extended the vault inactivity deadline")
	}
}

func TestCLIVaultChangeReencryptsLocalStateWithoutARemoteResult(t *testing.T) {
	service := newCLIVaultService(t)
	if err := service.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	const next = "another valid password"
	engine := connectEngine(t, ConnectHandlers{
		Secret: testCLISecret, Passwords: service,
		vault: newVaultOperations(service),
	})
	response := send(t, engine, http.MethodPost, testVaultChangePath,
		`{"current":"`+testPassphrase+`","next":"`+next+`"}`, cliHeaders(testCLISecret))
	if response.Code != http.StatusNoContent || response.Body.Len() != 0 {
		t.Fatalf("change = %d, want empty 204: %s", response.Code, response.Body.String())
	}
	service.Lock()
	if err := service.Unlock(next); err != nil {
		t.Fatalf("local password change was not committed: %v", err)
	}
}

type vaultLockSentinelPTY struct {
	*scriptedPTY
	closeCalled bool
}

func (p *vaultLockSentinelPTY) Close() error {
	p.closeCalled = true
	return nil
}

func TestCLIVaultLockDoesNotCloseLiveSessions(t *testing.T) {
	service := newCLIVaultService(t)
	if err := service.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	process := &vaultLockSentinelPTY{scriptedPTY: newScriptedPTY()}
	registry := &terminal.Registry{}
	if _, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindShell,
		Open: func(context.Context, terminal.Size) (terminal.Process, error) { return process, nil },
	}); err != nil {
		t.Fatal(err)
	}
	defer process.exit(terminal.ExitInfo{Code: 0})
	engine := connectEngine(t, ConnectHandlers{
		Secret: testCLISecret, Passwords: service,
		Sessions: func() int { return liveSessions(registry.Sessions()) },
	})
	if got := liveSessions(registry.Sessions()); got != 1 {
		t.Fatalf("sessions before lock = %d, want 1", got)
	}
	response := send(t, engine, http.MethodPost, testVaultLockPath, `{}`, cliHeaders(testCLISecret))
	if response.Code != http.StatusNoContent {
		t.Fatalf("lock = %d: %s", response.Code, response.Body.String())
	}
	if service.Unlocked() {
		t.Fatal("vault stayed unlocked")
	}
	if got := liveSessions(registry.Sessions()); got != 1 {
		t.Fatalf("sessions after lock = %d, want 1", got)
	}
	if process.closed || process.closeCalled {
		t.Fatalf("vault lock touched terminal process: hung up=%v closed=%v", process.closed, process.closeCalled)
	}
}

func TestLegacyCLIUnlockRouteIsNotRegistered(t *testing.T) {
	engine := connectEngine(t, ConnectHandlers{Secret: testCLISecret, Passwords: newCLIVaultService(t)})
	response := send(t, engine, http.MethodPost, "/cli/unlock", `{"passphrase":"anything at all"}`, cliHeaders(testCLISecret))
	if response.Code != http.StatusNotFound {
		t.Fatalf("legacy unlock = %d, want 404", response.Code)
	}
}

func newUnconfiguredSyncVaultServer(
	t *testing.T,
) (*Server, *secret.Service, session.Credentials) {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	manager := storage.NewManager(workspace, time.Now, rand.Reader)
	passwords := secret.NewService(workspace, manager, time.Now)
	if err := passwords.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	syncService := remotesync.NewService(workspace, manager, nil, nil)
	sessions, bootstrap, err := session.NewManager(bytes.NewReader(bytes.Repeat([]byte{0x68}, 96)))
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := sessions.Bootstrap(bootstrap)
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(Options{
		Listener:        fakeListener{address: &net.TCPAddr{IP: net.IP{127, 0, 0, 1}, Port: 43123}},
		CLISecret:       testCLISecret,
		Passwords:       passwords,
		Sync:            syncService,
		Sessions:        sessions,
		Owner:           handoff.OwnerEngine,
		Version:         "no-remote-test",
		ProtocolVersion: handoff.ProtocolVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	return server, passwords, credentials
}

func TestServerTreatsUnconfiguredRemoteAsSuccessfulCLILocalChange(t *testing.T) {
	server, passwords, _ := newUnconfiguredSyncVaultServer(t)
	const next = "replacement without a remote"
	request := httptest.NewRequest(http.MethodPost, VaultChangePath,
		strings.NewReader(`{"current":"`+testPassphrase+`","next":"`+next+`"}`))
	request.Host = "127.0.0.1:43123"
	request.Header.Set(echo.HeaderContentType, "application/json")
	request.Header.Set(handoff.HeaderName, testCLISecret)
	response := httptest.NewRecorder()
	server.http.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("CLI change = %d, want 204: %s", response.Code, response.Body.String())
	}
	passwords.Lock()
	if err := passwords.Unlock(next); err != nil {
		t.Fatalf("local change was not committed: %v", err)
	}
}

func TestServerPreservesBrowserShapeWhenRemoteIsUnconfigured(t *testing.T) {
	server, _, credentials := newUnconfiguredSyncVaultServer(t)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/passwords/change",
		strings.NewReader(`{"current":"`+testPassphrase+`","next":"replacement without a remote"}`))
	request.Host = "127.0.0.1:43123"
	request.Header.Set(echo.HeaderContentType, "application/json")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.Header.Set(echo.HeaderOrigin, "http://127.0.0.1:43123")
	request.Header.Set(CSRFHeader, credentials.CSRFToken)
	request.AddCookie(&http.Cookie{Name: SessionCookie, Value: credentials.SessionID})
	response := httptest.NewRecorder()
	server.http.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("browser change = %d, want 200: %s", response.Code, response.Body.String())
	}
	var answer api.ChangeMasterPasswordResult
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if !answer.Vault.Exists || !answer.Vault.Unlocked {
		t.Fatalf("local vault result = %+v", answer)
	}
}

type unreadVaultBody struct {
	reads int
}

func (b *unreadVaultBody) Read([]byte) (int, error) {
	b.reads++
	return 0, errors.New("body must not be read before authentication")
}

func (*unreadVaultBody) Close() error { return nil }

func TestUnauthenticatedVaultRoutesNeverReadTheBody(t *testing.T) {
	engine := connectEngine(t, ConnectHandlers{Secret: testCLISecret, Passwords: newCLIVaultService(t)})
	for _, path := range []string{VaultCreatePath, VaultUnlockPath, VaultLockPath, VaultChangePath} {
		t.Run(path, func(t *testing.T) {
			body := &unreadVaultBody{}
			request := httptest.NewRequest(http.MethodPost, path, nil)
			request.Body = body
			request.ContentLength = -1
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", response.Code)
			}
			if body.reads != 0 {
				t.Fatalf("unauthenticated request read body %d time(s)", body.reads)
			}
		})
	}
}

func TestVaultJSONLimitUsesTheReaderForEveryContentLength(t *testing.T) {
	service := newCLIVaultService(t)
	engine := connectEngine(t, ConnectHandlers{Secret: testCLISecret, Passwords: service})
	exact := "{}" + strings.Repeat(" ", maxVaultCLIBody-2)
	over := "{}" + strings.Repeat(" ", maxVaultCLIBody-1)
	tests := []struct {
		name          string
		body          string
		contentLength int64
		want          int
	}{
		{name: "exact declared", body: exact, contentLength: maxVaultCLIBody, want: http.StatusNoContent},
		{name: "over declared", body: over, contentLength: maxVaultCLIBody + 1, want: http.StatusRequestEntityTooLarge},
		{name: "over zero", body: over, contentLength: 0, want: http.StatusRequestEntityTooLarge},
		{name: "over unknown", body: over, contentLength: -1, want: http.StatusRequestEntityTooLarge},
		{name: "valid zero", body: `{}`, contentLength: 0, want: http.StatusNoContent},
		{name: "valid unknown", body: `{}`, contentLength: -1, want: http.StatusNoContent},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, VaultLockPath, strings.NewReader(test.body))
			request.ContentLength = test.contentLength
			request.Header.Set(handoff.HeaderName, testCLISecret)
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status = %d, want %d", response.Code, test.want)
			}
		})
	}
}

func TestVaultJSONRejectsEmptyAndNullBodies(t *testing.T) {
	engine := connectEngine(t, ConnectHandlers{Secret: testCLISecret, Passwords: newCLIVaultService(t)})
	for _, body := range []string{"", "null"} {
		request := httptest.NewRequest(http.MethodPost, VaultLockPath, strings.NewReader(body))
		request.Header.Set(handoff.HeaderName, testCLISecret)
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Errorf("body %q = %d, want 400", body, response.Code)
		}
	}
}

type vaultStateErrorFileSystem struct {
	storage.FileSystem
	err error
}

func (f *vaultStateErrorFileSystem) Lstat(path string) (fs.FileInfo, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.FileSystem.Lstat(path)
}

func (f *vaultStateErrorFileSystem) ReadFile(path string) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.FileSystem.ReadFile(path)
}

func newUnreadableVaultService(t *testing.T) *secret.Service {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	fileSystem := &vaultStateErrorFileSystem{FileSystem: storage.OSFileSystem{}}
	workspace, err := storage.NewWorkspace(fileSystem, home)
	if err != nil {
		t.Fatal(err)
	}
	fileSystem.err = errors.New("storage failed without secret details")
	return secret.NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader), time.Now)
}

func TestVaultStatusReturnsInternalErrorForMissingOrUnreadableService(t *testing.T) {
	tests := []struct {
		name    string
		service *secret.Service
	}{
		{name: "nil service"},
		{name: "storage error", service: newUnreadableVaultService(t)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cli := connectEngine(t, ConnectHandlers{Secret: testCLISecret, Passwords: test.service})
			for _, path := range []string{StatusPath, VaultStatusPath} {
				response := send(t, cli, http.MethodGet, path, "", cliHeaders(testCLISecret))
				if response.Code != http.StatusInternalServerError {
					t.Errorf("%s = %d, want 500", path, response.Code)
				}
				if strings.Contains(response.Body.String(), "storage failed") {
					t.Fatalf("%s leaked storage details: %s", path, response.Body.String())
				}
			}

			browser := echo.New()
			registerPasswordRoutes(browser, PasswordHandlers{Service: test.service})
			response := send(t, browser, http.MethodGet, "/api/v1/passwords", "", nil)
			if response.Code != http.StatusInternalServerError {
				t.Errorf("browser status = %d, want 500", response.Code)
			}
			if strings.Contains(response.Body.String(), "storage failed") {
				t.Fatalf("browser leaked storage details: %s", response.Body.String())
			}
		})
	}
}

func TestCLIVaultChangeWithoutAResealerIsLocalSuccess(t *testing.T) {
	service := newCLIVaultService(t)
	if err := service.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	engine := connectEngine(t, ConnectHandlers{Secret: testCLISecret, Passwords: service})
	response := send(t, engine, http.MethodPost, VaultChangePath,
		`{"current":"`+testPassphrase+`","next":"replacement without resealer"}`, cliHeaders(testCLISecret))
	if response.Code != http.StatusNoContent {
		t.Fatalf("change = %d, want 204: %s", response.Code, response.Body.String())
	}
}

func TestVaultMutationsReturnInternalErrorForNilService(t *testing.T) {
	cli := connectEngine(t, ConnectHandlers{Secret: testCLISecret})
	tests := []struct {
		path string
		body string
	}{
		{path: VaultCreatePath, body: `{"passphrase":"a valid master password"}`},
		{path: VaultUnlockPath, body: `{"passphrase":"a valid master password"}`},
		{path: VaultLockPath, body: `{}`},
		{path: VaultChangePath, body: `{"current":"a valid master password","next":"another valid password"}`},
	}
	for _, test := range tests {
		response := send(t, cli, http.MethodPost, test.path, test.body, cliHeaders(testCLISecret))
		if response.Code != http.StatusInternalServerError {
			t.Errorf("%s = %d, want 500", test.path, response.Code)
		}
		if strings.Contains(response.Body.String(), "master password") {
			t.Fatalf("%s leaked request data: %s", test.path, response.Body.String())
		}
	}
}

func TestVaultMutationsReturnInternalErrorForStorageFailure(t *testing.T) {
	service := newUnreadableVaultService(t)
	cli := connectEngine(t, ConnectHandlers{Secret: testCLISecret, Passwords: service})
	tests := []struct {
		path string
		body string
	}{
		{path: VaultCreatePath, body: `{"passphrase":"a valid master password"}`},
		{path: VaultUnlockPath, body: `{"passphrase":"a valid master password"}`},
		{path: VaultChangePath, body: `{"current":"a valid master password","next":"another valid password"}`},
	}
	for _, test := range tests {
		response := send(t, cli, http.MethodPost, test.path, test.body, cliHeaders(testCLISecret))
		if response.Code != http.StatusInternalServerError {
			t.Errorf("%s = %d, want 500", test.path, response.Code)
		}
		if strings.Contains(response.Body.String(), "storage failed") || strings.Contains(response.Body.String(), "master password") {
			t.Fatalf("%s leaked internal details: %s", test.path, response.Body.String())
		}
	}
}

var _ io.ReadCloser = (*unreadVaultBody)(nil)
