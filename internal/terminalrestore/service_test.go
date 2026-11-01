package terminalrestore_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"sshc/internal/platform/windowsacl/acltest"
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
	if err := newService(t, workspace).Save([]terminalrestore.Session{edge, local}); err != nil {
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
			if err := newService(t, workspace).Save([]terminalrestore.Session{edge}); err != nil {
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
	if err := newService(t, workspace).Save([]terminalrestore.Session{edge}); err != nil {
		t.Fatal(err)
	}
	if err := newService(t, workspace).Save([]terminalrestore.Session{local}); err != nil {
		t.Fatal(err)
	}
	if offer := newService(t, workspace).Offer(); !reflect.DeepEqual(offer, []terminalrestore.Session{edge, local}) {
		t.Fatalf("offer = %+v", offer)
	}
}

func TestStoppingWithNothingOpenLeavesNoRecord(t *testing.T) {
	workspace := newWorkspace(t)
	if err := newService(t, workspace).Save(nil); err != nil {
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
	acltest.WritePrivateFile(t, recordPath(workspace), []byte(damaged))
	service, err := terminalrestore.NewService(workspace)
	if !errors.Is(err, terminalrestore.ErrInvalidDocument) || len(service.Offer()) != 0 {
		t.Fatalf("NewService() = %+v, %v", service.Offer(), err)
	}
	if err := service.Save([]terminalrestore.Session{edge}); err != nil {
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

func TestAnOfferedSessionOpenedAgainByHandIsRecordedOnce(t *testing.T) {
	workspace := newWorkspace(t)
	second := edge
	second.PinnedTitle = "logs"
	if err := newService(t, workspace).Save([]terminalrestore.Session{edge, second, local}); err != nil {
		t.Fatal(err)
	}
	// The question was closed with Esc, and the user opened one session to
	// edge again by hand; it starts in the home directory with no name.
	byHand := terminalrestore.Session{Kind: terminal.KindSSH, Alias: "edge", Workspace: "default", WorkingDirectory: "~"}
	if err := newService(t, workspace).Save([]terminalrestore.Session{byHand}); err != nil {
		t.Fatal(err)
	}
	want := []terminalrestore.Session{second, local, byHand}
	if offer := newService(t, workspace).Offer(); !reflect.DeepEqual(offer, want) {
		t.Fatalf("offer = %+v, want %+v", offer, want)
	}
}

func TestEachEngineStartNamesItsOfferAnew(t *testing.T) {
	workspace := newWorkspace(t)
	if id := newService(t, workspace).OfferID(); id != "" {
		t.Fatalf("an engine with nothing to offer names an offer %q", id)
	}
	if err := newService(t, workspace).Save([]terminalrestore.Session{edge}); err != nil {
		t.Fatal(err)
	}
	first := newService(t, workspace)
	second := newService(t, workspace)
	if first.OfferID() == "" || first.OfferID() == second.OfferID() {
		t.Fatalf("offer ids of two starts = %q, %q; want two different ones", first.OfferID(), second.OfferID())
	}
	if err := second.Decline(); err != nil {
		t.Fatal(err)
	}
	if id := second.OfferID(); id != "" {
		t.Fatalf("an answered offer is still named %q", id)
	}
}

// The engine writes the record while it runs, so that one ended without stopping (a phone's system closing the app)
// still leaves what was open. What it wrote last is what the next engine offers.
func TestTheNextEngineOffersWhatWasRecordedLastWhileRunning(t *testing.T) {
	workspace := newWorkspace(t)
	running := newService(t, workspace)
	if err := running.Save([]terminalrestore.Session{edge}); err != nil {
		t.Fatal(err)
	}
	if err := running.Save([]terminalrestore.Session{edge, local}); err != nil {
		t.Fatal(err)
	}
	// The engine ends here, without stopping.
	if offer := newService(t, workspace).Offer(); !reflect.DeepEqual(offer, []terminalrestore.Session{edge, local}) {
		t.Fatalf("offer = %+v", offer)
	}
}

func TestTheSameSessionsAreNotWrittenAgain(t *testing.T) {
	workspace := newWorkspace(t)
	running := newService(t, workspace)
	if err := running.Save([]terminalrestore.Session{edge}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(recordPath(workspace)); err != nil {
		t.Fatal(err)
	}
	if err := running.Save([]terminalrestore.Session{edge}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(recordPath(workspace)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the same sessions were written again: %v", err)
	}
	if err := running.Save([]terminalrestore.Session{edge, local}); err != nil {
		t.Fatal(err)
	}
	if offer := newService(t, workspace).Offer(); !reflect.DeepEqual(offer, []terminalrestore.Session{edge, local}) {
		t.Fatalf("a change was not written: offer = %+v", offer)
	}
}

// An offer not yet answered stays in the record that the running engine keeps writing.
func TestAnUnansweredOfferStaysInTheRecordWrittenWhileRunning(t *testing.T) {
	workspace := newWorkspace(t)
	if err := newService(t, workspace).Save([]terminalrestore.Session{edge}); err != nil {
		t.Fatal(err)
	}
	running := newService(t, workspace)
	if err := running.Save([]terminalrestore.Session{local}); err != nil {
		t.Fatal(err)
	}
	if offer := newService(t, workspace).Offer(); !reflect.DeepEqual(offer, []terminalrestore.Session{edge, local}) {
		t.Fatalf("offer = %+v", offer)
	}
}
