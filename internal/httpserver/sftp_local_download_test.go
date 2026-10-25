package httpserver

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalDownloadHTTPSendsTheFileAsAnAttachmentAndRefusesLinks(t *testing.T) {
	fixture := newLocalMutationHTTPFixture(t)
	filename := filepath.Join(fixture.directory, "report.md")
	if err := os.WriteFile(filename, []byte("# report\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	endpoint := "/api/v1/sftp/local/download?path=" + url.QueryEscape(filepath.ToSlash(filename))
	response := sendKeyRequest(t, fixture.engine, fixture.credentials, http.MethodGet, endpoint, nil, "")
	if response.Code != http.StatusOK || response.Body.String() != "# report\n" {
		t.Fatalf("download = %d: %q", response.Code, response.Body.String())
	}
	header := response.Header()
	if header.Get("Content-Type") != "application/octet-stream" || header.Get("Content-Disposition") != `attachment; filename=report.md` || header.Get("ETag") == "" {
		t.Fatalf("headers = %v", header)
	}

	link := filepath.Join(fixture.directory, "link.md")
	if err := os.Symlink(filename, link); err != nil {
		t.Skipf("symlink: %v", err)
	}
	refused := sendKeyRequest(t, fixture.engine, fixture.credentials, http.MethodGet, "/api/v1/sftp/local/download?path="+url.QueryEscape(filepath.ToSlash(link)), nil, "")
	if refused.Code != http.StatusConflict {
		t.Fatalf("link = %d: %s", refused.Code, refused.Body.String())
	}
}
