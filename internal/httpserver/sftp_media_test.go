package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func issueMediaTicket(t *testing.T, fixture localMutationHTTPFixture, path string) *httptest.ResponseRecorder {
	t.Helper()
	body := mustMarshal(t, sftpMediaTicketRequest{Alias: "sshc://local", Path: filepath.ToSlash(path)})
	return sendKeyRequest(t, fixture.engine, fixture.credentials, http.MethodPost, "/api/v1/sftp/media-tickets", body, "")
}

// playerRequest is what <audio> and <video> send: the session cookie and the
// fetch metadata, but never the CSRF header.
func playerRequest(fixture localMutationHTTPFixture, ticket, byteRange string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/sftp/media/"+ticket, nil)
	request.Host = keyTestHost
	request.AddCookie(&http.Cookie{Name: SessionCookie, Value: fixture.credentials.SessionID})
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	if byteRange != "" {
		request.Header.Set("Range", byteRange)
	}
	response := httptest.NewRecorder()
	fixture.engine.ServeHTTP(response, request)
	return response
}

func TestThePlayerReadsTheRangesItAsksForWithATicketInsteadOfTheCSRFHeader(t *testing.T) {
	fixture := newLocalMutationHTTPFixture(t)
	filename := filepath.Join(fixture.directory, "take.mp3")
	if err := os.WriteFile(filename, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	issued := issueMediaTicket(t, fixture, filename)
	if issued.Code != http.StatusCreated {
		t.Fatalf("issue = %d: %s", issued.Code, issued.Body.String())
	}
	var ticket sftpMediaTicketResponse
	if err := json.Unmarshal(issued.Body.Bytes(), &ticket); err != nil {
		t.Fatal(err)
	}

	// Safari probes with a closed range before it plays.
	probe := playerRequest(fixture, ticket.Ticket, "bytes=0-1")
	if probe.Code != http.StatusPartialContent || probe.Body.String() != "01" || probe.Header().Get("Content-Range") != "bytes 0-1/10" {
		t.Fatalf("probe = %d %q %v", probe.Code, probe.Body.String(), probe.Header())
	}
	if probe.Header().Get("Content-Type") != "audio/mpeg" || probe.Header().Get("Accept-Ranges") != "bytes" {
		t.Fatalf("headers = %v", probe.Header())
	}
	seek := playerRequest(fixture, ticket.Ticket, "bytes=7-")
	if seek.Code != http.StatusPartialContent || seek.Body.String() != "789" {
		t.Fatalf("seek = %d %q", seek.Code, seek.Body.String())
	}

	if unknown := playerRequest(fixture, ticket.Ticket+"x", ""); unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown ticket = %d", unknown.Code)
	}
	revoked := sendKeyRequest(t, fixture.engine, fixture.credentials, http.MethodDelete, "/api/v1/sftp/media-tickets/"+ticket.Ticket, nil, "")
	if revoked.Code != http.StatusNoContent {
		t.Fatalf("revoke = %d", revoked.Code)
	}
	if after := playerRequest(fixture, ticket.Ticket, ""); after.Code != http.StatusNotFound {
		t.Fatalf("after revoke = %d", after.Code)
	}
}

func TestOnlyFilesThatPlayGetAMediaTicket(t *testing.T) {
	fixture := newLocalMutationHTTPFixture(t)
	if refused := issueMediaTicket(t, fixture, filepath.Join(fixture.directory, "notes.txt")); refused.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("text = %d: %s", refused.Code, refused.Body.String())
	}
}

func TestAMissingFileGetsNoMediaTicketAndSaysItIsNotFound(t *testing.T) {
	fixture := newLocalMutationHTTPFixture(t)
	refused := issueMediaTicket(t, fixture, filepath.Join(fixture.directory, "IMG_2041.jpgcording.mp4"))
	if refused.Code != http.StatusNotFound || !strings.Contains(refused.Body.String(), "sftp_not_found") {
		t.Fatalf("missing = %d: %s", refused.Code, refused.Body.String())
	}
}

func TestAMediaTicketWorksOnlyForTheSessionThatAskedForIt(t *testing.T) {
	tickets := &mediaTickets{}
	token, err := tickets.issue("first", "edge", "/clip.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tickets.claim("second", token); ok {
		t.Fatal("another session claimed the ticket")
	}
	tickets.revoke("second", token)
	if claim, ok := tickets.claim("first", token); !ok || claim.alias != "edge" || claim.path != "/clip.mp4" {
		t.Fatalf("claim = %+v, %v", claim, ok)
	}
}

func TestASessionKeepsOnlyItsNewestMediaTickets(t *testing.T) {
	tickets := &mediaTickets{}
	oldest, err := tickets.issue("viewer", "edge", "/0.mp4")
	if err != nil {
		t.Fatal(err)
	}
	for range maxMediaTicketsPerSession {
		if _, err := tickets.issue("viewer", "edge", "/next.mp4"); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := tickets.claim("viewer", oldest); ok {
		t.Fatal("the oldest ticket outlived the limit")
	}
	if _, err := tickets.issue("other", "edge", "/other.mp4"); err != nil {
		t.Fatal(err)
	}
}
