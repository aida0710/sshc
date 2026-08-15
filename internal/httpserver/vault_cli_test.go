package httpserver

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sshc/internal/handoff"
	"sshc/internal/remotesync"
	"sshc/internal/secret"
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
		Owner: handoff.OwnerHeadless, Version: "v-status-test", ProtocolVersion: handoff.ProtocolVersion,
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
		if answer.Owner != handoff.OwnerHeadless || answer.Version != "v-status-test" || answer.ProtocolVersion != handoff.ProtocolVersion {
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

func TestCLIVaultChangeReturnsStructuredPartialSuccess(t *testing.T) {
	service := newCLIVaultService(t)
	if err := service.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	const next = "another valid password"
	engine := connectEngine(t, ConnectHandlers{
		Secret: testCLISecret, Passwords: service,
		ResealSnapshot: func(_ context.Context, passphrase string) error {
			if passphrase != next {
				t.Fatalf("reseal passphrase = %q", passphrase)
			}
			return remotesync.ErrPushRefused
		},
	})
	response := send(t, engine, http.MethodPost, testVaultChangePath,
		`{"current":"`+testPassphrase+`","next":"`+next+`"}`, cliHeaders(testCLISecret))
	if response.Code != http.StatusMultiStatus {
		t.Fatalf("change = %d, want 207: %s", response.Code, response.Body.String())
	}
	var answer struct {
		SnapshotResealed bool    `json:"snapshotResealed"`
		SnapshotProblem  *string `json:"snapshotProblem"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if answer.SnapshotResealed || answer.SnapshotProblem == nil || *answer.SnapshotProblem != "sync_push_refused" {
		t.Fatalf("partial result = %+v", answer)
	}
	if strings.Contains(response.Body.String(), testPassphrase) || strings.Contains(response.Body.String(), next) {
		t.Fatal("partial response contains an entered password")
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
	if _, err := registry.Open(terminal.Spec{
		Kind: terminal.KindShell,
		Open: func(terminal.Size) (terminal.Process, error) { return process, nil },
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
