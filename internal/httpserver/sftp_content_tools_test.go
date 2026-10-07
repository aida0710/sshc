package httpserver

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"testing"

	"github.com/labstack/echo/v5"
	"sshc/internal/api"
	"sshc/internal/session"
	sshcSFTP "sshc/internal/sftp"
)

type httpContentRemote struct {
	sshcSFTP.Remote
	contents map[string]string
	reads    int
}

func (*httpContentRemote) Close() error { return nil }
func (remote *httpContentRemote) Lstat(candidate string) (fs.FileInfo, error) {
	if candidate == "/" || candidate == "/work" {
		return remoteMetadataInfo{name: path.Base(candidate), mode: fs.ModeDir | 0o755}, nil
	}
	contents, exists := remote.contents[candidate]
	if !exists {
		return nil, fs.ErrNotExist
	}
	return remoteMetadataInfo{name: path.Base(candidate), mode: 0o644, size: int64(len(contents))}, nil
}
func (remote *httpContentRemote) ReadDir(_ context.Context, directory string) ([]fs.FileInfo, error) {
	var entries []fs.FileInfo
	for candidate := range remote.contents {
		if path.Dir(candidate) != directory {
			continue
		}
		info, err := remote.Lstat(candidate)
		if err != nil {
			return nil, err
		}
		entries = append(entries, info)
	}
	return entries, nil
}
func (remote *httpContentRemote) Open(candidate string) (io.ReadCloser, error) {
	info, err := remote.Lstat(candidate)
	if err != nil {
		return nil, err
	}
	remote.reads++
	return httpContentFile{Reader: bytes.NewReader([]byte(remote.contents[candidate])), metadata: info}, nil
}

type httpContentFile struct {
	*bytes.Reader
	metadata fs.FileInfo
}

func (file httpContentFile) Stat() (fs.FileInfo, error) { return file.metadata, nil }
func (httpContentFile) Close() error                    { return nil }

func contentHTTPEngine(t *testing.T) (*echo.Echo, *httpContentRemote, session.Credentials) {
	t.Helper()
	manager, token, err := session.NewManager(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	credentials, _, err := manager.BootstrapForSession(token, "")
	if err != nil {
		t.Fatal(err)
	}
	remote := &httpContentRemote{contents: map[string]string{"/work/notes.txt": "first\nneedle\n", "/work/binary": "needle\x00"}}
	service := &sshcSFTP.Service{Open: func(context.Context, string) (sshcSFTP.Remote, error) { return remote, nil }}
	engine := echo.New()
	engine.Use((Security{ExpectedHost: keyTestHost, ExpectedOrigin: "http://" + keyTestHost, Sessions: manager, Unlocked: alwaysUnlocked}).Middleware)
	registerSFTPRoutes(engine, SFTPHandlers{Service: service})
	return engine, remote, credentials
}

func TestContentSearchHTTPReturnsLinesOmissionsAndPreservesDefaultNameSearch(t *testing.T) {
	engine, remote, credentials := contentHTTPEngine(t)
	response := sendKeyRequest(t, engine, credentials, http.MethodGet, "/api/v1/sftp/edge/search?path=/work&query=needle&mode=content", nil, "")
	if response.Code != http.StatusOK {
		t.Fatalf("search = %d %s", response.Code, response.Body.String())
	}
	var found sftpSearchResponse
	if err := json.Unmarshal(response.Body.Bytes(), &found); err != nil {
		t.Fatal(err)
	}
	if found.Matches == nil || len(*found.Matches) != 1 || (*found.Matches)[0].Line != 2 || (*found.Matches)[0].Entry.Path != "/work/notes.txt" || found.Omissions == nil || !found.Truncated || found.BytesRead == nil {
		t.Fatalf("content wire = %+v", found)
	}
	reads := remote.reads
	response = sendKeyRequest(t, engine, credentials, http.MethodGet, "/api/v1/sftp/edge/search?path=/work&query=notes", nil, "")
	if response.Code != http.StatusOK {
		t.Fatalf("names = %d %s", response.Code, response.Body.String())
	}
	var names sftpSearchResponse
	if err := json.Unmarshal(response.Body.Bytes(), &names); err != nil {
		t.Fatal(err)
	}
	if len(names.Entries) != 1 || names.Matches != nil || remote.reads != reads {
		t.Fatalf("name mode read contents or changed shape: %+v", names)
	}
}

func TestContentToolsHTTPRejectsMissingAuthorizationAndInvalidModesBeforeReading(t *testing.T) {
	engine, remote, credentials := contentHTTPEngine(t)
	endpoints := []string{
		"/api/v1/sftp/edge/search?path=/work&query=needle&mode=content",
		"/api/v1/sftp/compare?leftAlias=edge&leftPath=/work&rightAlias=other&rightPath=/work&mode=content",
	}
	for _, endpoint := range endpoints {
		response := sendKeyRequest(t, engine, session.Credentials{}, http.MethodGet, endpoint, nil, "")
		if response.Code < http.StatusUnauthorized || remote.reads != 0 {
			t.Fatalf("unauthorized read = %d, %d", response.Code, remote.reads)
		}
		response = sendKeyRequest(t, engine, credentials, http.MethodGet, endpoint+"-invalid", nil, "")
		if response.Code != http.StatusBadRequest || remote.reads != 0 {
			t.Fatalf("invalid mode = %d %s", response.Code, response.Body.String())
		}
	}
}

func TestOpeningAContentMatchHTTPPinsTheReadToItsRevision(t *testing.T) {
	engine, remote, credentials := contentHTTPEngine(t)
	response := sendKeyRequest(t, engine, credentials, http.MethodGet, "/api/v1/sftp/edge/search?path=/work&query=needle&mode=content", nil, "")
	var found sftpSearchResponse
	if err := json.Unmarshal(response.Body.Bytes(), &found); err != nil || found.Matches == nil || len(*found.Matches) != 1 {
		t.Fatalf("search match = %s, %v", response.Body.String(), err)
	}
	revision := (*found.Matches)[0].Entry.Revision
	endpoint := "/api/v1/sftp/edge/text?path=/work/notes.txt&expectedRevision=" + url.QueryEscape(revision)
	response = sendKeyRequest(t, engine, credentials, http.MethodGet, endpoint, nil, "")
	if response.Code != http.StatusOK {
		t.Fatalf("open matching revision = %d %s", response.Code, response.Body.String())
	}
	reads := remote.reads
	remote.contents["/work/notes.txt"] = "replacement with a different size"
	response = sendKeyRequest(t, engine, credentials, http.MethodGet, endpoint, nil, "")
	if response.Code != http.StatusConflict || remote.reads != reads {
		t.Fatalf("changed match read = %d, %d after %d", response.Code, remote.reads, reads)
	}
}

func TestContentComparisonHTTPReportsItsModeAndMeasuredReads(t *testing.T) {
	engine, _, credentials := contentHTTPEngine(t)
	endpoint := "/api/v1/sftp/compare?leftAlias=edge&leftPath=/work&rightAlias=other&rightPath=/work&mode=content"
	response := sendKeyRequest(t, engine, credentials, http.MethodGet, endpoint, nil, "")
	if response.Code != http.StatusOK {
		t.Fatalf("comparison = %d %s", response.Code, response.Body.String())
	}
	var comparison api.SFTPDirectoryComparison
	if err := json.Unmarshal(response.Body.Bytes(), &comparison); err != nil {
		t.Fatal(err)
	}
	if comparison.Mode == nil || string(*comparison.Mode) != "content" || comparison.BytesRead == nil || *comparison.BytesRead != 2*int64(len("first\nneedle\n")+len("needle\x00")) || comparison.Truncated == nil || *comparison.Truncated {
		t.Fatalf("content comparison wire = %+v", comparison)
	}
}
