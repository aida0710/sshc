package httpserver

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshc/internal/api"
	"sshc/internal/terminal"
)

func TestLocalShellStartsInTheRequestedFolderWithOrWithoutAProfile(t *testing.T) {
	for _, profile := range []string{"", "default"} {
		t.Run("profile="+profile, func(t *testing.T) {
			fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 2, Scrollback: 1 << 12})
			directory := filepath.Join(t.TempDir(), "project's $(whoami); & work")
			if err := os.Mkdir(directory, 0o700); err != nil {
				t.Fatal(err)
			}
			request := map[string]string{"kind": "shell", "cwd": filepath.ToSlash(directory)}
			if profile != "" {
				request["profileId"] = profile
			}
			body, err := json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			response, contents := fixture.do(t, http.MethodPost, "/api/v1/terminal/sessions", string(body))
			if response.StatusCode != http.StatusCreated {
				t.Fatalf("open = %d: %s", response.StatusCode, contents)
			}
			var opened api.OpenTerminalSessionResponse
			if err := json.Unmarshal([]byte(contents), &opened); err != nil {
				t.Fatal(err)
			}
			if opened.Session.Kind != api.TerminalSessionKindShell || opened.Session.Alias != nil {
				t.Fatalf("local folder opened as SSH: %#v", opened.Session)
			}
			commands := fixture.starter.opened()
			if len(commands) != 1 || commands[0].Dir != directory {
				t.Fatalf("commands = %#v; want literal working directory %q", commands, directory)
			}
			if strings.Contains(strings.Join(commands[0].Arguments, " "), filepath.Base(directory)) {
				t.Fatal("the working directory reached shell arguments")
			}
			if writes := fixture.starter.last().keystrokes(); writes != "" {
				t.Fatalf("shell startup input = %q; want no cd command", writes)
			}
		})
	}
}

func TestLocalShellRefusesAnUnavailableFolderWithoutStartingOrFallingBack(t *testing.T) {
	for _, profile := range []string{"", "default"} {
		t.Run("profile="+profile, func(t *testing.T) {
			fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 2, Scrollback: 1 << 12})
			request := map[string]string{"kind": "shell", "cwd": filepath.ToSlash(filepath.Join(t.TempDir(), "missing"))}
			if profile != "" {
				request["profileId"] = profile
			}
			body, err := json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			response, contents := fixture.do(t, http.MethodPost, "/api/v1/terminal/sessions", string(body))
			if response.StatusCode != http.StatusBadRequest || !strings.Contains(contents, "local_working_directory_unavailable") {
				t.Fatalf("open = %d: %s", response.StatusCode, contents)
			}
			if commands := fixture.starter.opened(); len(commands) != 0 {
				t.Fatalf("unavailable folder started a shell: %#v", commands)
			}
		})
	}
}
