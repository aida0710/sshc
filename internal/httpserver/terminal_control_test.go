package httpserver

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"sshc/internal/api"
	"sshc/internal/terminal"
)

func TestTerminalControlReadsBoundedPlainTextWithACursor(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 2, Scrollback: 64})
	id, _ := fixture.openShell(t)
	process := fixture.starter.last()
	process.feed("\x1b[31mfirst\x1b[0m\r\nsecond")
	session, _ := fixture.registry.Lookup(id)
	waitUntil(t, func() bool { return strings.Contains(string(session.Snapshot()), "second") })

	response, body := fixture.do(t, http.MethodGet,
		"/api/v1/terminal/sessions/"+id+"/control?cursor=0&limit=5", "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("control = %d: %s", response.StatusCode, body)
	}
	var first api.TerminalControlResponse
	if err := json.Unmarshal([]byte(body), &first); err != nil {
		t.Fatal(err)
	}
	if strings.ContainsRune(first.Output, '\x1b') || first.Cursor.Next != 5 || first.Generation == 0 || string(first.State) != string(terminal.ControlConnected) {
		t.Fatalf("first control response = %#v", first)
	}

	response, body = fixture.do(t, http.MethodGet,
		"/api/v1/terminal/sessions/"+id+"/control?cursor="+strconvFormat(first.Cursor.Next)+"&limit=64", "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("second control = %d: %s", response.StatusCode, body)
	}
	var second api.TerminalControlResponse
	if err := json.Unmarshal([]byte(body), &second); err != nil {
		t.Fatal(err)
	}
	if second.Cursor.Start != first.Cursor.Next || second.Cursor.Next <= second.Cursor.Start || strings.ContainsRune(second.Output, '\x1b') {
		t.Fatalf("second control response = %#v", second)
	}
}

func TestTerminalControlRejectsInvalidOrFutureCursors(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 1, Scrollback: 64})
	id, _ := fixture.openShell(t)
	for _, test := range []struct {
		query  string
		status int
	}{
		{"cursor=bad", http.StatusBadRequest},
		{"limit=65537", http.StatusBadRequest},
		{"cursor=1", http.StatusConflict},
	} {
		response, _ := fixture.do(t, http.MethodGet, "/api/v1/terminal/sessions/"+id+"/control?"+test.query, "")
		if response.StatusCode != test.status {
			t.Errorf("%s = %d, want %d", test.query, response.StatusCode, test.status)
		}
	}
}

func TestTerminalControlCarriesCSIAndOSCAcrossCursorBoundaries(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 1, Scrollback: 256})
	id, _ := fixture.openShell(t)
	process := fixture.starter.last()
	raw := "prefix\x1b[31mred\x1b[0m\x1b]8;;https://secret.invalid\x1b\\link\x1b]8;;\x1b\\after"
	process.feed(raw)
	session, _ := fixture.registry.Lookup(id)
	waitUntil(t, func() bool { return len(session.Snapshot()) == len(raw) })

	// This cursor is in the OSC payload. The response must decode from retained
	// context, rather than exposing the remaining URL as ordinary text.
	cursor := len("prefix\x1b[31mred\x1b[0m\x1b]8;;https://sec")
	response, body := fixture.do(t, http.MethodGet,
		"/api/v1/terminal/sessions/"+id+"/control?cursor="+strconv.Itoa(cursor)+"&limit=128", "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("control = %d: %s", response.StatusCode, body)
	}
	var control api.TerminalControlResponse
	if err := json.Unmarshal([]byte(body), &control); err != nil {
		t.Fatal(err)
	}
	if control.Output != "linkafter" || strings.Contains(control.Output, "secret") || strings.ContainsRune(control.Output, '\x1b') {
		t.Fatalf("fragmented OSC output = %q", control.Output)
	}
}

func strconvFormat(value uint64) string {
	return strconv.FormatUint(value, 10)
}
