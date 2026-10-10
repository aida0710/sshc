package httpserver

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sshcSFTP "sshc/internal/sftp"
)

func TestLocalTextHTTPReadsAndSavesWithoutOpeningSSH(t *testing.T) {
	fixture := newLocalMutationHTTPFixture(t)
	filename := filepath.Join(fixture.directory, "notes.txt")
	if err := os.WriteFile(filename, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	endpoint := "/api/v1/sftp/local/text?path=" + url.QueryEscape(filepath.ToSlash(filename))
	read := sendKeyRequest(t, fixture.engine, fixture.credentials, http.MethodGet, endpoint, nil, "")
	if read.Code != http.StatusOK {
		t.Fatalf("read = %d: %s", read.Code, read.Body.String())
	}
	var opened sftpTextFileResponse
	if err := json.Unmarshal(read.Body.Bytes(), &opened); err != nil {
		t.Fatal(err)
	}
	for _, revision := range []string{"", "stale", opened.Revision} {
		response := sendKeyRequest(t, fixture.engine, fixture.credentials, http.MethodPut, endpoint, mustMarshal(t, sftpSaveTextRequest{Contents: "after", ExpectedRevision: revision}), "")
		want := http.StatusOK
		if revision == "" {
			want = http.StatusBadRequest
		} else if revision == "stale" {
			want = http.StatusConflict
		}
		if response.Code != want {
			t.Fatalf("save %q = %d: %s; want %d", revision, response.Code, response.Body.String(), want)
		}
	}
	contents, err := os.ReadFile(filename)
	if err != nil || string(contents) != "after" {
		t.Fatalf("saved = %q, %v", contents, err)
	}
}

func TestLocalTextHTTPAcceptsEscapedTextAtEditorLimitAndRefusesMore(t *testing.T) {
	fixture := newLocalMutationHTTPFixture(t)
	filename := filepath.Join(fixture.directory, "large.txt")
	if err := os.WriteFile(filename, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	opened, err := sshcSFTP.ReadLocalText(t.Context(), sshcSFTP.LocalTextReadOptions{Path: filename})
	if err != nil {
		t.Fatal(err)
	}
	endpoint := "/api/v1/sftp/local/text?path=" + url.QueryEscape(filepath.ToSlash(filename))
	contents := strings.Repeat("\n\"", sshcSFTP.MaxEditableFileBytes/2)
	body := mustMarshal(t, sftpSaveTextRequest{Contents: contents, ExpectedRevision: opened.Revision})
	if len(body) <= MaxRequestBodyCeiling {
		t.Fatal("fixture did not exceed the default JSON ceiling")
	}
	response := sendKeyRequest(t, fixture.engine, fixture.credentials, http.MethodPut, endpoint, body, "")
	if response.Code != http.StatusOK {
		t.Fatalf("limit save = %d: %s", response.Code, response.Body.String())
	}
	var saved sftpTextFileResponse
	if err := json.Unmarshal(response.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Contents != contents {
		t.Fatal("escaped contents changed")
	}
	response = sendKeyRequest(t, fixture.engine, fixture.credentials, http.MethodPut, endpoint, mustMarshal(t, sftpSaveTextRequest{Contents: contents + "x", ExpectedRevision: saved.Revision}), "")
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("over limit = %d", response.Code)
	}
}
