package terminalrestore_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"sshc/internal/storage"
	"sshc/internal/terminal"
	"sshc/internal/terminalrestore"
	"sshc/internal/terminalworkspace"
)

func newWorkspace(t *testing.T) *storage.Workspace {
	t.Helper()
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return workspace
}

func recordPath(workspace *storage.Workspace) string {
	return filepath.Join(workspace.Root(), filepath.FromSlash(terminalrestore.PathRelative))
}

func newService(t *testing.T, workspace *storage.Workspace) *terminalrestore.Service {
	t.Helper()
	service, err := terminalrestore.NewService(workspace)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

var (
	edge  = terminalrestore.Session{Kind: terminal.KindSSH, Alias: "edge", Workspace: "default", WorkingDirectory: "~/src"}
	local = terminalrestore.Session{Kind: terminal.KindShell, ShellProfile: "zsh", Workspace: "work", PinnedTitle: "build", WorkingDirectory: "/home/aida"}
)

func TestTheNextEngineOffersTheSessionsOpenAtStop(t *testing.T) {
	workspace := newWorkspace(t)
	if offer := newService(t, workspace).Offer(); len(offer) != 0 {
		t.Fatalf("a first start offers %+v", offer)
	}
	if err := newService(t, workspace).SaveOnStop([]terminalrestore.Session{edge, local}); err != nil {
		t.Fatal(err)
	}
	if offer := newService(t, workspace).Offer(); !reflect.DeepEqual(offer, []terminalrestore.Session{edge, local}) {
		t.Fatalf("offer = %+v", offer)
	}
}

func TestAnsweringEndsTheOfferForEveryLaterStart(t *testing.T) {
	for name, answer := range map[string]func(*terminalrestore.Service) error{
		"take":    func(service *terminalrestore.Service) error { _, err := service.Take(); return err },
		"decline": (*terminalrestore.Service).Decline,
	} {
		t.Run(name, func(t *testing.T) {
			workspace := newWorkspace(t)
			if err := newService(t, workspace).SaveOnStop([]terminalrestore.Session{edge}); err != nil {
				t.Fatal(err)
			}
			service := newService(t, workspace)
			if err := answer(service); err != nil {
				t.Fatal(err)
			}
			if offer := service.Offer(); len(offer) != 0 {
				t.Fatalf("offer after answering = %+v", offer)
			}
			if _, err := os.Stat(recordPath(workspace)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("the record survived the answer: %v", err)
			}
			if offer := newService(t, workspace).Offer(); len(offer) != 0 {
				t.Fatalf("a later start offers %+v", offer)
			}
		})
	}
}

func TestStoppingBeforeAnsweringKeepsTheOffer(t *testing.T) {
	workspace := newWorkspace(t)
	if err := newService(t, workspace).SaveOnStop([]terminalrestore.Session{edge}); err != nil {
		t.Fatal(err)
	}
	if err := newService(t, workspace).SaveOnStop([]terminalrestore.Session{local}); err != nil {
		t.Fatal(err)
	}
	if offer := newService(t, workspace).Offer(); !reflect.DeepEqual(offer, []terminalrestore.Session{edge, local}) {
		t.Fatalf("offer = %+v", offer)
	}
}

func TestStoppingWithNothingOpenLeavesNoRecord(t *testing.T) {
	workspace := newWorkspace(t)
	if err := newService(t, workspace).SaveOnStop(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(recordPath(workspace)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stat record = %v", err)
	}
}

func TestADamagedRecordOffersNothingAndIsReplacedAtStop(t *testing.T) {
	workspace := newWorkspace(t)
	if err := workspace.EnsureDirectory(workspace.StateDir()); err != nil {
		t.Fatal(err)
	}
	damaged := `{"schemaVersion":1,"sessions":[{"kind":"ssh","alias":"-oProxyCommand=x","workspace":"default"}]}`
	if err := os.WriteFile(recordPath(workspace), []byte(damaged), 0o600); err != nil {
		t.Fatal(err)
	}
	service, err := terminalrestore.NewService(workspace)
	if !errors.Is(err, terminalrestore.ErrInvalidDocument) || len(service.Offer()) != 0 {
		t.Fatalf("NewService() = %+v, %v", service.Offer(), err)
	}
	if err := service.SaveOnStop([]terminalrestore.Session{edge}); err != nil {
		t.Fatal(err)
	}
	if offer := newService(t, workspace).Offer(); !reflect.DeepEqual(offer, []terminalrestore.Session{edge}) {
		t.Fatalf("offer = %+v", offer)
	}
}

func TestAgentSessionsAreNotOpenedAgain(t *testing.T) {
	sessions := terminalrestore.FromReopenings([]terminal.Reopening{
		{Kind: terminal.KindSSH, Alias: "edge", Workspace: terminalworkspace.DefaultID, WorkingDirectory: "~/src"},
		{Kind: terminal.KindShell, ShellProfile: "zsh", Workspace: terminalworkspace.AgentsID},
	})
	want := []terminalrestore.Session{{Kind: terminal.KindSSH, Alias: "edge", Workspace: terminalworkspace.DefaultID, WorkingDirectory: "~/src"}}
	if !reflect.DeepEqual(sessions, want) {
		t.Fatalf("FromReopenings() = %+v", sessions)
	}
}
