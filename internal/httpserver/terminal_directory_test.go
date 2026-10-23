package httpserver

import (
	"net/http"
	"strings"
	"testing"

	"sshc/internal/terminal"
)

func TestTerminalDirectorySaysWhenTheSessionCannotAnswer(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 1, Scrollback: 64})
	id, _ := fixture.openShell(t)

	// The scripted process is not a pseudo-terminal that can name its directory.
	response, body := fixture.do(t, http.MethodGet, "/api/v1/terminal/sessions/"+id+"/directory", "")
	if response.StatusCode != http.StatusConflict || !strings.Contains(body, "terminal_directory_unavailable") {
		t.Fatalf("directory = %d: %s", response.StatusCode, body)
	}
	response, body = fixture.do(t, http.MethodGet, "/api/v1/terminal/sessions/missing/directory", "")
	if response.StatusCode != http.StatusNotFound || !strings.Contains(body, "terminal_session_not_found") {
		t.Fatalf("missing session directory = %d: %s", response.StatusCode, body)
	}
}
