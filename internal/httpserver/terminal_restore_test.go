package httpserver

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"sshc/internal/api"
	"sshc/internal/terminal"
	"sshc/internal/terminalrestore"
	"sshc/internal/testwait"
)

func TestRemoteDirectoriesUnderHomeAreChangedIntoWithTheTildeUnquoted(t *testing.T) {
	cases := map[string]string{
		"~":                "cd",
		"~/src/it's":       `cd -- ~/'src/it'"'"'s'`,
		"/srv/project dir": `cd -- '/srv/project dir'`,
	}
	for directory, want := range cases {
		accepted, err := remoteWorkingDirectory(directory)
		if err != nil {
			t.Fatalf("remoteWorkingDirectory(%q) = %v", directory, err)
		}
		if got := changeRemoteDirectory(accepted); got != want {
			t.Errorf("changeRemoteDirectory(%q) = %q, want %q", directory, got, want)
		}
	}
}

func restoreOffer(t *testing.T, fixture *terminalFixture) api.TerminalRestoreOffer {
	t.Helper()
	response, body := fixture.do(t, http.MethodGet, "/api/v1/terminal/restore", "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("offer = %d: %s", response.StatusCode, body)
	}
	var offer api.TerminalRestoreOffer
	if err := json.Unmarshal([]byte(body), &offer); err != nil {
		t.Fatal(err)
	}
	return offer
}

func TestRestoreOpensTheOfferedSessionsAndLetsGoOfWhatChanged(t *testing.T) {
	present := t.TempDir()
	missing := filepath.Join(t.TempDir(), "removed")
	fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 3, Scrollback: 1 << 12},
		terminalrestore.Session{Kind: terminal.KindSSH, Alias: "edge", Workspace: "default", PinnedTitle: "deploy", WorkingDirectory: "~/src"},
		terminalrestore.Session{Kind: terminal.KindShell, ShellProfile: "zsh", Workspace: "removed-workspace", WorkingDirectory: filepath.ToSlash(present)},
		terminalrestore.Session{Kind: terminal.KindShell, Workspace: "default", WorkingDirectory: filepath.ToSlash(missing)},
		terminalrestore.Session{Kind: terminal.KindSSH, Alias: "over-the-limit", Workspace: "default"},
	)
	offer := restoreOffer(t, fixture)
	if len(offer.Sessions) != 4 || offer.Sessions[0].Alias == nil || *offer.Sessions[0].Alias != "edge" ||
		offer.Sessions[0].WorkingDirectory == nil || *offer.Sessions[0].WorkingDirectory != "~/src" {
		t.Fatalf("offer = %+v", offer)
	}

	response, body := fixture.do(t, http.MethodPost, "/api/v1/terminal/restore", "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("restore = %d: %s", response.StatusCode, body)
	}
	var result api.TerminalRestoreResult
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatal(err)
	}
	if result.Opened != 3 || result.Failed != 1 {
		t.Fatalf("result = %+v, want 3 opened and 1 over the limit", result)
	}

	sessions := fixture.registry.Sessions()
	if len(sessions) != 3 {
		t.Fatalf("sessions = %+v", sessions)
	}
	if ssh := sessions[0]; ssh.Alias != "edge" || ssh.Title != "deploy" || ssh.Workspace != "default" {
		t.Fatalf("ssh session = %+v", ssh)
	}
	if moved := sessions[1]; moved.Kind != terminal.KindShell || moved.Workspace != "default" || moved.ShellProfile != "default" {
		t.Fatalf("a session of a removed workspace or profile = %+v", moved)
	}
	commands := fixture.starter.opened()
	if len(commands) != 2 || commands[0].Dir != present || commands[1].Dir != "/home/tester" {
		t.Fatalf("local directories = %+v", commands)
	}
	if offer := restoreOffer(t, fixture); len(offer.Sessions) != 0 {
		t.Fatalf("the offer survived the restore: %+v", offer)
	}
}

func TestDecliningARestoreOpensNothingAndEndsTheOffer(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 3, Scrollback: 1 << 12},
		terminalrestore.Session{Kind: terminal.KindShell, Workspace: "default"},
	)
	response, body := fixture.do(t, http.MethodDelete, "/api/v1/terminal/restore", "")
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("decline = %d: %s", response.StatusCode, body)
	}
	if sessions := fixture.registry.Sessions(); len(sessions) != 0 {
		t.Fatalf("declining opened %+v", sessions)
	}
	if offer := restoreOffer(t, fixture); len(offer.Sessions) != 0 {
		t.Fatalf("the offer survived declining: %+v", offer)
	}
}

func TestTheSessionListTellsWhereEachSessionWorks(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 2, Scrollback: 1 << 12})
	directory := t.TempDir()
	if err := os.Mkdir(filepath.Join(directory, "app"), 0o700); err != nil {
		t.Fatal(err)
	}
	fixture.openShell(t)
	fixture.starter.last().feed("\x1b]7;file://edge" + filepath.ToSlash(filepath.Join(directory, "app")) + "\a")
	var listed api.TerminalSessionList
	testwait.Until(t, func() bool {
		_, body := fixture.do(t, http.MethodGet, "/api/v1/terminal/sessions", "")
		if err := json.Unmarshal([]byte(body), &listed); err != nil {
			t.Fatal(err)
		}
		return len(listed.Sessions) == 1 && listed.Sessions[0].WorkingDirectory != nil
	})
	session := listed.Sessions[0]
	if *session.WorkingDirectory != filepath.ToSlash(filepath.Join(directory, "app")) || session.ShellProfileId == nil || *session.ShellProfileId != "default" {
		t.Fatalf("listed session = %+v", session)
	}
}
