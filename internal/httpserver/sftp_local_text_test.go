package httpserver

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
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
