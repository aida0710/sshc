package httpserver

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v5"
	"sshc/internal/api"
	"sshc/internal/session"
	sshcSFTP "sshc/internal/sftp"
)

type localMutationHTTPFixture struct {
	engine      *echo.Echo
	credentials session.Credentials
	directory   string
}

type localMutationHTTPRequest struct {
	path  string
	body  any
	token string
}

func newLocalMutationHTTPFixture(t *testing.T) localMutationHTTPFixture {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	directory := filepath.Join(home, "files")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	service := &sshcSFTP.Service{Open: func(context.Context, string) (sshcSFTP.Remote, error) {
		t.Error("local operation opened a remote")
		return nil, sshcSFTP.ErrUnavailable
	}}
	transfers := sshcSFTP.NewTransferManager(service, t.TempDir())
	t.Cleanup(func() { _ = transfers.Close() })
	sessions, bootstrap, err := session.NewManager(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	credentials, _, err := sessions.BootstrapForSession(bootstrap, "")
	if err != nil {
		t.Fatal(err)
	}
	engine := echo.New()
	engine.Use((Security{ExpectedHost: keyTestHost, ExpectedOrigin: "http://" + keyTestHost, Sessions: sessions, Unlocked: alwaysUnlocked}).Middleware)
	registerSFTPRoutes(engine, SFTPHandlers{Service: service, Transfers: transfers, Actions: ActionHandlers{Sessions: sessions}})
	return localMutationHTTPFixture{engine: engine, credentials: credentials, directory: directory}
}

func (fixture localMutationHTTPFixture) send(t *testing.T, request localMutationHTTPRequest) *httptest.ResponseRecorder {
	t.Helper()
	return sendKeyRequest(t, fixture.engine, fixture.credentials, http.MethodPost, "/api/v1/sftp/local/"+request.path, mustMarshal(t, request.body), request.token)
}

func (fixture localMutationHTTPFixture) selection(t *testing.T, names ...string) []api.SFTPLocalDeleteEntry {
	t.Helper()
	listing, err := sshcSFTP.ListLocal(fixture.directory)
	if err != nil {
		t.Fatal(err)
	}
	selection := make([]api.SFTPLocalDeleteEntry, 0, len(names))
	for _, name := range names {
		found := false
		for _, entry := range listing.Entries {
			if entry.Name == name {
				selection = append(selection, api.SFTPLocalDeleteEntry{Path: entry.Path, ExpectedRevision: entry.Revision})
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing fixture %s", name)
		}
	}
	return selection
}

func (fixture localMutationHTTPFixture) plan(t *testing.T, selection []api.SFTPLocalDeleteEntry) api.SFTPLocalDeletePlan {
	t.Helper()
	response := fixture.send(t, localMutationHTTPRequest{path: "delete-plan", body: api.SFTPLocalDeleteSelection{Entries: selection}})
	if response.Code != http.StatusOK {
		t.Fatalf("plan = %d: %s", response.Code, response.Body.String())
	}
	var plan api.SFTPLocalDeletePlan
	if err := json.Unmarshal(response.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestLocalMutationHTTPUsesDedicatedRoutesAndRevisionChecks(t *testing.T) {
	fixture := newLocalMutationHTTPFixture(t)
	created := fixture.send(t, localMutationHTTPRequest{path: "directories", body: api.SFTPLocalMkdirRequest{Directory: fixture.directory, Name: "folder"}})
	if created.Code != http.StatusCreated {
		t.Fatalf("mkdir = %d: %s", created.Code, created.Body.String())
	}
	var entry api.SFTPEntry
	if err := json.Unmarshal(created.Body.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	for _, request := range []api.SFTPLocalRenameRequest{
		{Path: entry.Path, Name: "renamed", ExpectedRevision: "stale"},
		{Path: entry.Path, Name: "../escape", ExpectedRevision: entry.Revision},
	} {
		response := fixture.send(t, localMutationHTTPRequest{path: "rename", body: request})
		if response.Code != http.StatusConflict && response.Code != http.StatusBadRequest {
			t.Fatalf("refused rename = %d: %s", response.Code, response.Body.String())
		}
	}
	renamed := fixture.send(t, localMutationHTTPRequest{path: "rename", body: api.SFTPLocalRenameRequest{Path: entry.Path, Name: "renamed", ExpectedRevision: entry.Revision}})
	if renamed.Code != http.StatusOK {
		t.Fatalf("rename = %d: %s", renamed.Code, renamed.Body.String())
	}
	if _, err := os.Stat(filepath.Join(fixture.directory, "renamed")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(fixture.directory, "folder")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("source = %v", err)
	}
}

func TestLocalDeleteHTTPRequiresAPlanTokenAndConsumesItOnce(t *testing.T) {
	fixture := newLocalMutationHTTPFixture(t)
	filename := filepath.Join(fixture.directory, "notes.txt")
	if err := os.WriteFile(filename, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	selection := fixture.selection(t, "notes.txt")
	plan := fixture.plan(t, selection)
	body := api.SFTPLocalDeleteRequest{Entries: selection, ExpectedRevision: plan.Revision}
	missing := fixture.send(t, localMutationHTTPRequest{path: "delete", body: body})
	if missing.Code != http.StatusForbidden || problemCode(t, missing.Body.Bytes()) != "action_token_required" {
		t.Fatalf("missing token = %d: %s", missing.Code, missing.Body.String())
	}
	deleted := fixture.send(t, localMutationHTTPRequest{path: "delete", body: body, token: plan.ActionToken})
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete = %d: %s", deleted.Code, deleted.Body.String())
	}
	if err := os.WriteFile(filename, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	selection = fixture.selection(t, "notes.txt")
	nextPlan := fixture.plan(t, selection)
	replayed := fixture.send(t, localMutationHTTPRequest{path: "delete", body: api.SFTPLocalDeleteRequest{Entries: selection, ExpectedRevision: nextPlan.Revision}, token: plan.ActionToken})
	if replayed.Code != http.StatusForbidden || problemCode(t, replayed.Body.Bytes()) != "action_token_invalid" {
		t.Fatalf("replay = %d: %s", replayed.Code, replayed.Body.String())
	}
	if contents, err := os.ReadFile(filename); err != nil || string(contents) != "new" {
		t.Fatalf("replay changed file: %q, %v", contents, err)
	}
}

func TestLocalDeleteHTTPBindsTheTokenToTheEntireSelection(t *testing.T) {
	fixture := newLocalMutationHTTPFixture(t)
	for _, name := range []string{"one.txt", "two.txt"} {
		if err := os.WriteFile(filepath.Join(fixture.directory, name), []byte("keep"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	first := fixture.plan(t, fixture.selection(t, "one.txt"))
	selection := fixture.selection(t, "two.txt")
	second := fixture.plan(t, selection)
	refused := fixture.send(t, localMutationHTTPRequest{path: "delete", body: api.SFTPLocalDeleteRequest{Entries: selection, ExpectedRevision: second.Revision}, token: first.ActionToken})
	if refused.Code != http.StatusForbidden {
		t.Fatalf("other selection = %d: %s", refused.Code, refused.Body.String())
	}
	for _, name := range []string{"one.txt", "two.txt"} {
		if _, err := os.Stat(filepath.Join(fixture.directory, name)); err != nil {
			t.Fatal(err)
		}
	}
	selection = fixture.selection(t, "one.txt", "two.txt")
	combined := fixture.plan(t, selection)
	deleted := fixture.send(t, localMutationHTTPRequest{path: "delete", body: api.SFTPLocalDeleteRequest{Entries: selection, ExpectedRevision: combined.Revision}, token: combined.ActionToken})
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("multi delete = %d: %s", deleted.Code, deleted.Body.String())
	}
}

func TestLocalDeleteHTTPRejectsChangedDescendantsAndLeavesAllSelections(t *testing.T) {
	fixture := newLocalMutationHTTPFixture(t)
	folder := filepath.Join(fixture.directory, "folder")
	if err := os.Mkdir(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(folder, "notes.txt")
	other := filepath.Join(fixture.directory, "aaa.txt")
	for _, name := range []string{child, other} {
		if err := os.WriteFile(name, []byte("keep"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	selection := fixture.selection(t, "aaa.txt", "folder")
	plan := fixture.plan(t, selection)
	if err := os.WriteFile(child, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	response := fixture.send(t, localMutationHTTPRequest{path: "delete", body: api.SFTPLocalDeleteRequest{Entries: selection, ExpectedRevision: plan.Revision}, token: plan.ActionToken})
	if response.Code != http.StatusConflict || problemCode(t, response.Body.Bytes()) != "sftp_conflict" {
		t.Fatalf("changed tree = %d: %s", response.Code, response.Body.String())
	}
	for _, name := range []string{child, other} {
		if _, err := os.Stat(name); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLocalMutationHTTPRejectsUnknownInputAndMissingAuthentication(t *testing.T) {
	fixture := newLocalMutationHTTPFixture(t)
	invalid := fixture.send(t, localMutationHTTPRequest{path: "directories", body: map[string]any{"directory": fixture.directory, "name": "folder", "overwrite": true}})
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("unknown field = %d: %s", invalid.Code, invalid.Body.String())
	}
	for _, route := range []string{"directories", "rename", "delete-plan", "delete"} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/sftp/local/"+route, nil)
		request.Host = keyTestHost
		request.Header.Set("Sec-Fetch-Site", "same-origin")
		request.Header.Set("Origin", "http://"+keyTestHost)
		fixture.engine.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Errorf("unauthenticated %s = %d", route, response.Code)
		}
	}
}
