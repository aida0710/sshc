package application

import (
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"sshc/internal/snippets"
)

type startupRemovalHarness struct {
	connectionUpdateHarness
	library *snippets.Service
}

func newStartupRemovalHarness(t *testing.T, contents string, startupAliases ...string) startupRemovalHarness {
	t.Helper()
	harness := newConnectionUpdateHarness(t, contents)
	store := snippets.NewStore(harness.workspace, snippets.Protection{
		Seal: harness.secrets.SealDocument, Open: harness.secrets.OpenDocument,
		WithMutation: harness.secrets.WithStableSnapshot,
	})
	harness.service.SetStartupRenamer(store)
	harness.service.SetStartupRemover(store)
	library := snippets.NewService(snippets.Options{
		Repository: store, Now: time.Now, Random: rand.Reader,
		Resolve: func(alias string) (snippets.Resolution, error) {
			return snippets.Resolution{
				Target:  snippets.Target{Alias: alias, HostName: alias + ".example", Port: "22"},
				Binding: "destination-of-" + alias,
			}, nil
		},
	})
	snippet, err := library.Create(snippets.Draft{Name: "Startup", Command: "echo ready"})
	if err != nil {
		t.Fatal(err)
	}
	for _, alias := range startupAliases {
		if err := library.SetStartup(alias, snippet.ID, nil); err != nil {
			t.Fatal(err)
		}
	}
	return startupRemovalHarness{connectionUpdateHarness: harness, library: library}
}

func (h startupRemovalHarness) hasStartup(t *testing.T, alias string) bool {
	t.Helper()
	_, err := h.library.PrepareStartupCommand(alias)
	if errors.Is(err, snippets.ErrNoStartup) {
		return false
	}
	if err != nil {
		t.Fatalf("startup for %s: %v", alias, err)
	}
	return true
}

func TestDeletingHostDropsItsStartupSoARecreatedAliasDoesNotInheritIt(t *testing.T) {
	const before = "Host edge\n\tHostName edge.example\n\nHost other\n\tHostName other.example\n"
	harness := newStartupRemovalHarness(t, before, "edge", "other")

	const deleted = "Host other\n\tHostName other.example\n"
	if _, err := harness.service.SaveWithSecrets(EditRequest{
		Kind: EditFileRaw, Path: "config", Base: before, Raw: deleted,
	}); err != nil {
		t.Fatal(err)
	}
	if harness.hasStartup(t, "edge") {
		t.Fatal("deleting edge kept its startup snippet")
	}
	if !harness.hasStartup(t, "other") {
		t.Fatal("deleting edge dropped the startup snippet of other")
	}

	const recreated = deleted + "\nHost edge\n\tHostName elsewhere.example\n"
	if _, err := harness.service.SaveWithSecrets(EditRequest{
		Kind: EditFileRaw, Path: "config", Base: deleted, Raw: recreated,
	}); err != nil {
		t.Fatal(err)
	}
	if harness.hasStartup(t, "edge") {
		t.Fatal("a host recreated under a deleted alias inherited its startup snippet")
	}
}

func TestReplacingHostLineInBlockDropsStartupOfTheAliasItNoLongerDeclares(t *testing.T) {
	const before = "Host edge\n\tHostName edge.example\n"
	harness := newStartupRemovalHarness(t, before, "edge")

	if _, err := harness.service.SaveWithSecrets(EditRequest{
		Kind: EditBlockRaw, Path: "config", Base: before, Alias: "edge",
		Raw: "Host replaced\n\tHostName edge.example\n",
	}); err != nil {
		t.Fatal(err)
	}
	if harness.hasStartup(t, "edge") {
		t.Fatal("the alias removed from the Host line kept its startup snippet")
	}
}

func TestRawEditThatKeepsTheHostDeclarationKeepsItsStartup(t *testing.T) {
	const before = "Host edge\n\tHostName edge.example\n"
	harness := newStartupRemovalHarness(t, before, "edge")

	if _, err := harness.service.SaveWithSecrets(EditRequest{
		Kind: EditBlockRaw, Path: "config", Base: before, Alias: "edge",
		Raw: "Host edge\n\tHostName edge.example\n\tPort 2222\n",
	}); err != nil {
		t.Fatal(err)
	}
	if !harness.hasStartup(t, "edge") {
		t.Fatal("editing edge without removing it dropped its startup snippet")
	}
}
