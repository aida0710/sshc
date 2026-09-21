package application

import (
	"bytes"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sshc/internal/secret"
	"sshc/internal/snippets"
	"sshc/internal/storage"
)

func TestFailedAliasRenameRestoresConfigSecretsAndStartup(t *testing.T) {
	const before = "Host edge\n\tHostName edge.example\n\tPort 22\n"
	for _, failedPath := range []string{secret.WorkspacePath, "sshc/snippets.json"} {
		t.Run(failedPath, func(t *testing.T) {
			harness := newConnectionUpdateHarness(t, before)
			setPasswordForCurrentTarget(t, harness.service, harness.secrets, "edge", "original-password")
			fileSystem := &failRenameOnceFileSystem{FileSystem: storage.OSFileSystem{}}
			workspace, err := storage.NewWorkspace(fileSystem, harness.workspace.Home())
			if err != nil {
				t.Fatal(err)
			}
			manager := storage.NewManager(workspace, time.Now, rand.Reader)
			manager.Seal, manager.Unseal = harness.secrets.SealBackup, harness.secrets.OpenBackup
			service := NewService(workspace, manager)
			store := snippets.NewStore(workspace, snippets.Protection{
				Seal: harness.secrets.SealDocument, Open: harness.secrets.OpenDocument,
				WithMutation: harness.secrets.WithStableSnapshot,
			})
			service.SetStartupRenamer(store)
			library := snippets.NewService(snippets.Options{
				Repository: store, Now: time.Now, Random: rand.Reader,
				Resolve: func(alias string) (snippets.Resolution, error) {
					return snippets.Resolution{Target: snippets.Target{Alias: alias, HostName: "edge.example", Port: "22"}}, nil
				},
			})
			snippet, err := library.Create(snippets.Draft{Name: "Startup", Command: "echo ready"})
			if err != nil {
				t.Fatal(err)
			}
			if err := library.SetStartup("edge", snippet.ID, nil); err != nil {
				t.Fatal(err)
			}
			baselines := make(map[string][]byte)
			for _, relative := range []string{"config", secret.WorkspacePath, "sshc/snippets.json"} {
				path := filepath.Join(workspace.Root(), filepath.FromSlash(relative))
				baselines[path], err = os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
			}
			injected := errors.New("injected alias commit failure")
			fileSystem.path, fileSystem.err = filepath.Join(workspace.Root(), filepath.FromSlash(failedPath)), injected
			_, err = service.SaveWithSecrets(harness.secrets, EditRequest{
				Kind: EditRename, Path: "config", Base: before, Alias: "edge", NewAlias: "renamed",
			})
			if !errors.Is(err, injected) {
				t.Fatalf("rename error = %v", err)
			}
			for path, original := range baselines {
				current, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(current, original) {
					t.Errorf("failed rename changed %s: %v", filepath.Base(path), err)
				}
			}
			if got := passwordForCurrentTarget(t, service, harness.secrets, "edge"); got != "original-password" {
				t.Fatal("failed rename published the new vault in memory")
			}
			if _, err := library.PrepareStartupCommand("edge"); err != nil {
				t.Fatalf("failed rename lost startup binding: %v", err)
			}
			pending, err := manager.Pending()
			if err != nil || len(pending) != 0 {
				t.Fatalf("rollback left pending transactions: %v, %v", pending, err)
			}
		})
	}
}
