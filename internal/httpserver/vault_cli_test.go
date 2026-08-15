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
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/handoff"
	"sshc/internal/objectstore"
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
		vault: newVaultOperations(service, func(_ context.Context, passphrase string) error {
			if passphrase != next {
				t.Fatalf("reseal passphrase = %q", passphrase)
			}
			return remotesync.ErrPushRefused
		}),
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

func TestVaultOperationsSerializeLocalChangeThroughRemoteReseal(t *testing.T) {
	service := newCLIVaultService(t)
	service.SetSleep(func(time.Duration) {})
	if err := service.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	const first = "first replacement password"
	const second = "second replacement password"
	firstReseal := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondReseal := make(chan struct{})
	var remoteMu sync.Mutex
	remotePassword := ""
	operations := newVaultOperations(service, func(_ context.Context, passphrase string) error {
		remoteMu.Lock()
		remotePassword = passphrase
		remoteMu.Unlock()
		switch passphrase {
		case first:
			close(firstReseal)
			<-releaseFirst
		case second:
			close(secondReseal)
		}
		return nil
	})

	firstDone := make(chan error, 1)
	go func() {
		_, err := operations.Change(context.Background(), testPassphrase, first)
		firstDone <- err
	}()
	<-firstReseal

	secondCalling := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		close(secondCalling)
		_, err := operations.Change(context.Background(), first, second)
		secondDone <- err
	}()
	<-secondCalling
	select {
	case <-secondReseal:
		t.Fatal("second reseal overtook the first reseal")
	case err := <-secondDone:
		t.Fatalf("second change completed before the first reseal: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatalf("first change: %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second change: %v", err)
	}
	select {
	case <-secondReseal:
	default:
		t.Fatal("second reseal did not run after the first")
	}
	remoteMu.Lock()
	gotRemote := remotePassword
	remoteMu.Unlock()
	if gotRemote != second {
		t.Fatalf("remote password = %q, want final generation", gotRemote)
	}
	service.Lock()
	if err := service.Unlock(first); !errors.Is(err, secret.ErrWrongPassphrase) {
		t.Fatalf("first replacement still unlocks: %v", err)
	}
	if err := service.Unlock(second); err != nil {
		t.Fatalf("final local password does not unlock: %v", err)
	}
}

func TestVaultLockWaitsForRemoteReseal(t *testing.T) {
	service := newCLIVaultService(t)
	if err := service.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	resealStarted := make(chan struct{})
	releaseReseal := make(chan struct{})
	operations := newVaultOperations(service, func(context.Context, string) error {
		close(resealStarted)
		<-releaseReseal
		return nil
	})
	changeDone := make(chan error, 1)
	go func() {
		_, err := operations.Change(context.Background(), testPassphrase, "replacement password")
		changeDone <- err
	}()
	<-resealStarted

	lockCalling := make(chan struct{})
	lockDone := make(chan struct{})
	go func() {
		close(lockCalling)
		operations.Lock()
		close(lockDone)
	}()
	<-lockCalling
	select {
	case <-lockDone:
		t.Fatal("vault lock interleaved before remote reseal finished")
	case <-time.After(100 * time.Millisecond):
	}
	if !service.Unlocked() {
		t.Fatal("vault locked while remote reseal was still running")
	}

	close(releaseReseal)
	if err := <-changeDone; err != nil {
		t.Fatal(err)
	}
	<-lockDone
	if service.Unlocked() {
		t.Fatal("vault remained unlocked after the ordered lock")
	}
}

func TestCanceledVaultChangeDoesNotRunAfterWaitingForReseal(t *testing.T) {
	service := newCLIVaultService(t)
	if err := service.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	resealStarted := make(chan struct{})
	releaseReseal := make(chan struct{})
	var reseals int
	var resealMu sync.Mutex
	operations := newVaultOperations(service, func(context.Context, string) error {
		resealMu.Lock()
		reseals++
		current := reseals
		resealMu.Unlock()
		if current == 1 {
			close(resealStarted)
			<-releaseReseal
		}
		return nil
	})
	firstDone := make(chan error, 1)
	go func() {
		_, err := operations.Change(context.Background(), testPassphrase, "first replacement password")
		firstDone <- err
	}()
	<-resealStarted

	ctx, cancel := context.WithCancel(context.Background())
	secondDone := make(chan error, 1)
	go func() {
		_, err := operations.Change(ctx, "first replacement password", "canceled replacement password")
		secondDone <- err
	}()
	cancel()
	close(releaseReseal)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err := <-secondDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled change = %v, want context.Canceled", err)
	}
	resealMu.Lock()
	gotReseals := reseals
	resealMu.Unlock()
	if gotReseals != 1 {
		t.Fatalf("canceled waiter ran %d reseals", gotReseals)
	}
	service.Lock()
	if err := service.Unlock("first replacement password"); err != nil {
		t.Fatalf("canceled waiter changed the local password: %v", err)
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
	syncService := remotesync.NewService(workspace, manager, nil, nil, nil)
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
		Owner:           handoff.OwnerHeadless,
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
	if answer.SnapshotResealed || answer.SnapshotProblem != nil {
		t.Fatalf("unconfigured remote result = %+v", answer)
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

type blockingVaultResealBucket struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *blockingVaultResealBucket) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPut {
		response.WriteHeader(http.StatusNotFound)
		return
	}
	b.once.Do(func() { close(b.entered) })
	<-b.release
	_, _ = io.Copy(io.Discard, request.Body)
	response.Header().Set("ETag", `"resealed"`)
	response.WriteHeader(http.StatusOK)
}

func TestServerSharesCoordinatorBetweenCLIChangeAndBrowserLock(t *testing.T) {
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
	bucket := &blockingVaultResealBucket{entered: make(chan struct{}), release: make(chan struct{})}
	bucketServer := httptest.NewTLSServer(bucket)
	t.Cleanup(bucketServer.Close)
	credentials := objectstore.Credentials{AccessKeyID: "AKID", SecretAccessKey: "secret"}
	syncService := remotesync.NewService(
		workspace,
		manager,
		func() ([]string, error) { return nil, nil },
		func() string { return "2026-08-15T00:00:00Z" },
		func() (string, error) { return "vault-order-test", nil },
	)
	syncService.Configure(
		remotesync.Config{Endpoint: bucketServer.URL, Bucket: "sshc", Region: "auto"},
		credentials,
		&objectstore.Client{
			HTTP: bucketServer.Client(), Endpoint: bucketServer.URL,
			Bucket: "sshc", Region: "auto", Creds: credentials,
		},
	)
	sessions, bootstrap, err := session.NewManager(bytes.NewReader(bytes.Repeat([]byte{0x69}, 96)))
	if err != nil {
		t.Fatal(err)
	}
	browserCredentials, err := sessions.Bootstrap(bootstrap)
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(Options{
		Listener:        fakeListener{address: &net.TCPAddr{IP: net.IP{127, 0, 0, 1}, Port: 43123}},
		CLISecret:       testCLISecret,
		Passwords:       passwords,
		Sync:            syncService,
		Sessions:        sessions,
		Owner:           handoff.OwnerHeadless,
		Version:         "coordinator-test",
		ProtocolVersion: handoff.ProtocolVersion,
	})
	if err != nil {
		t.Fatal(err)
	}

	changeDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		request := httptest.NewRequest(http.MethodPost, VaultChangePath,
			strings.NewReader(`{"current":"`+testPassphrase+`","next":"ordered replacement password"}`))
		request.Host = "127.0.0.1:43123"
		request.Header.Set(echo.HeaderContentType, "application/json")
		request.Header.Set(handoff.HeaderName, testCLISecret)
		response := httptest.NewRecorder()
		server.http.Handler.ServeHTTP(response, request)
		changeDone <- response
	}()
	<-bucket.entered

	lockCalling := make(chan struct{})
	lockDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/passwords/lock", strings.NewReader(`{}`))
		request.Host = "127.0.0.1:43123"
		request.Header.Set(echo.HeaderContentType, "application/json")
		request.Header.Set("Sec-Fetch-Site", "same-origin")
		request.Header.Set(echo.HeaderOrigin, "http://127.0.0.1:43123")
		request.Header.Set(CSRFHeader, browserCredentials.CSRFToken)
		request.AddCookie(&http.Cookie{Name: SessionCookie, Value: browserCredentials.SessionID})
		response := httptest.NewRecorder()
		close(lockCalling)
		server.http.Handler.ServeHTTP(response, request)
		lockDone <- response
	}()
	<-lockCalling
	select {
	case response := <-lockDone:
		t.Fatalf("browser lock completed during CLI reseal: %d", response.Code)
	case <-time.After(100 * time.Millisecond):
	}
	if !passwords.Unlocked() {
		t.Fatal("browser lock interleaved with CLI reseal")
	}

	close(bucket.release)
	if response := <-changeDone; response.Code != http.StatusNoContent {
		t.Fatalf("CLI change = %d: %s", response.Code, response.Body.String())
	}
	if response := <-lockDone; response.Code != http.StatusOK {
		t.Fatalf("browser lock = %d: %s", response.Code, response.Body.String())
	}
	if passwords.Unlocked() {
		t.Fatal("ordered browser lock did not run after CLI reseal")
	}
}
