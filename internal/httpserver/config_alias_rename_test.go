package httpserver

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"sshc/internal/application"
	"sshc/internal/secret"
	"sshc/internal/snippets"
	"sshc/internal/storage"
)

func TestFailedCredentialRenameDoesNotCommitTheConfig(t *testing.T) {
	harness := newConfigHarness(t)
	secrets := secret.NewService(harness.workspace, storage.NewManager(harness.workspace, time.Now, rand.Reader), time.Now)
	if err := secrets.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	if err := secrets.SetBound("bastion", "synthetic-password", testPasswordBinding); err != nil {
		t.Fatal(err)
	}
	// Another service publishes a valid new generation after the first one has opened it.
	other := secret.NewService(harness.workspace, storage.NewManager(harness.workspace, time.Now, rand.Reader), time.Now)
	if err := other.Unlock(testPassphrase); err != nil {
		t.Fatal(err)
	}
	if err := other.SetBound("other", "synthetic-other-password", testPasswordBinding); err != nil {
		t.Fatal(err)
	}
	handler := ConfigHandlers{Service: harness.service, Secrets: secrets}
	body, err := json.Marshal(application.EditRequest{Kind: application.EditRename, Path: "config", Base: handlerConfig, Alias: "bastion", NewAlias: "edge"})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/config/save", bytes.NewReader(body))
	request.Header.Set(echo.HeaderContentType, "application/json")
	response := httptest.NewRecorder()
	if err := handler.Save(harness.echo.NewContext(request, response)); err != nil {
		t.Fatal(err)
	}
	if response.Code < 400 {
		t.Fatalf("expected credential write conflict, got %d", response.Code)
	}
	contents, err := os.ReadFile(filepath.Join(harness.root, "config"))
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != handlerConfig {
		t.Fatalf("HTTP %d reports failure, but the config changed", response.Code)
	}
	if !secrets.HasAssignmentFor(secret.KindPassword, "bastion") || secrets.HasAssignmentFor(secret.KindPassword, "edge") {
		t.Fatal("failed rename changed the vault assignments")
	}
}

func TestRenamingAHostCarriesItsStartupSnippet(t *testing.T) {
	harness := newConfigHarness(t)
	secrets := secret.NewService(harness.workspace, storage.NewManager(harness.workspace, time.Now, rand.Reader), time.Now)
	if err := secrets.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	store := snippets.NewStore(harness.workspace, snippets.Protection{
		Seal: secrets.SealDocument, Open: secrets.OpenDocument, WithMutation: secrets.WithStableSnapshot,
	})
	harness.service.SetStartupRenamer(store)
	service := snippets.NewService(snippets.Options{Repository: store, Now: time.Now, Random: rand.Reader,
		Resolve: func(alias string) (snippets.Resolution, error) {
			return snippets.Resolution{Target: snippets.Target{Alias: alias, HostName: "server.example", User: "ops", Port: "22"}}, nil
		},
	})
	created, err := service.Create(snippets.Draft{Name: "Startup", Command: "echo audit"})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.SetStartup("bastion", created.ID, nil); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(application.EditRequest{Kind: application.EditRename, Path: "config", Base: handlerConfig, Alias: "bastion", NewAlias: "edge"})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/config/save", bytes.NewReader(body))
	request.Header.Set(echo.HeaderContentType, "application/json")
	response := httptest.NewRecorder()
	if err := (ConfigHandlers{Service: harness.service, Secrets: secrets}).Save(harness.echo.NewContext(request, response)); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK {
		t.Fatalf("rename: %d", response.Code)
	}
	if _, err := service.PrepareStartupCommand("bastion"); err == nil {
		t.Error("old alias still owns an executable startup command")
	}
	if _, err := service.PrepareStartupCommand("edge"); err != nil {
		t.Errorf("renamed host lost startup command: %v", err)
	}
}
