package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshc/internal/httpserver"
)

func testSFTPEngine(server *httptest.Server) *engineAPI {
	return &engineAPI{
		origin: server.URL, csrf: "csrf", cookie: http.Cookie{Name: httpserver.SessionCookie, Value: "session"}, client: server.Client(),
	}
}

func TestSFTPDownloadUsesRemotePathAndPublishesAtomically(t *testing.T) {
	var createdRemotePath string
	var checkpointOffset float64
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/sftp/transfers":
			var body map[string]any
			_ = json.NewDecoder(request.Body).Decode(&body)
			createdRemotePath, _ = body["remotePath"].(string)
			writeTestJSON(response, http.StatusCreated, map[string]any{})
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/actions"):
			writeTestJSON(response, http.StatusOK, map[string]any{})
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/download"):
			if request.URL.Query().Get("path") != "/remote/file.txt" {
				t.Errorf("download path = %q", request.URL.Query().Get("path"))
			}
			response.Header().Set("ETag", `"revision-one"`)
			response.Header().Set("Content-Length", "3")
			_, _ = io.WriteString(response, "new")
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/download-checkpoint"):
			var body map[string]any
			_ = json.NewDecoder(request.Body).Decode(&body)
			checkpointOffset, _ = body["offset"].(float64)
			writeTestJSON(response, http.StatusOK, map[string]any{})
		default:
			t.Errorf("unexpected request: %s %s", request.Method, request.URL.String())
			response.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	destination := filepath.Join(t.TempDir(), "file.txt")
	err := sftpDownloadFile(context.Background(), testSFTPEngine(server), "server-a", "batch_12345678", sftpCLIFile{
		Source: "/remote/file.txt", Destination: destination, Size: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(destination)
	if err != nil || string(contents) != "new" {
		t.Fatalf("downloaded = %q, %v", contents, err)
	}
	if createdRemotePath != "/remote/file.txt" || checkpointOffset != 3 {
		t.Fatalf("job remotePath=%q checkpoint=%v", createdRemotePath, checkpointOffset)
	}
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(destination), ".sshc-sftp-*"))
	if len(matches) != 0 {
		t.Fatalf("temporary files remain: %v", matches)
	}
}

func TestSFTPUploadStreamsChunksAndCarriesOverwritePolicy(t *testing.T) {
	var uploaded strings.Builder
	var overwrite bool
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/sftp/transfers":
			var body map[string]any
			_ = json.NewDecoder(request.Body).Decode(&body)
			overwrite, _ = body["overwrite"].(bool)
			writeTestJSON(response, http.StatusCreated, map[string]any{})
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/actions"):
			writeTestJSON(response, http.StatusOK, map[string]any{})
		case request.Method == http.MethodPost && strings.Contains(request.URL.Path, "/uploads/") && !strings.HasSuffix(request.URL.Path, "/complete"):
			writeTestJSON(response, http.StatusOK, map[string]any{
				"id": "put_12345678", "path": "/remote/file.txt", "offset": 0, "size": 7, "expectedRevision": "missing:rev",
			})
		case request.Method == http.MethodPatch && strings.Contains(request.URL.Path, "/uploads/"):
			contents, _ := io.ReadAll(request.Body)
			uploaded.Write(contents)
			writeTestJSON(response, http.StatusOK, map[string]any{
				"id": "put_12345678", "path": "/remote/file.txt", "offset": len(contents), "size": 7, "expectedRevision": "",
			})
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/complete"):
			writeTestJSON(response, http.StatusCreated, map[string]any{"path": "/remote/file.txt", "bytes": 7, "revision": "revision-two"})
		default:
			t.Errorf("unexpected request: %s %s", request.Method, request.URL.String())
			response.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	source := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(source, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := sftpUploadFile(context.Background(), testSFTPEngine(server), "server-a", "batch_12345678", sftpCLIFile{
		Source: source, Destination: "/remote/file.txt", Size: 7,
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if uploaded.String() != "payload" || !overwrite {
		t.Fatalf("uploaded=%q overwrite=%v", uploaded.String(), overwrite)
	}
}

func TestSFTPFingerprintMatchesTheBrowserTreeHash(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "fingerprint")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString("abc"); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	got, err := sftpFingerprint(context.Background(), file, 3)
	if err != nil {
		t.Fatal(err)
	}
	const want = "tree-sha256:e13dee54bfc1b26042c1fec4b1d8ef2054b22fa1ab1263adce665eb013f829d5"
	if got != want {
		t.Fatalf("fingerprint = %q, want %q", got, want)
	}
}

func writeTestJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
