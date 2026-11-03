package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	sshcSFTP "sshc/internal/sftp"
)

func thumbnailTestPNG(t *testing.T) []byte {
	t.Helper()
	var encoded bytes.Buffer
	picture := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for index := range picture.Pix {
		picture.Pix[index] = 0xFF
	}
	if err := png.Encode(&encoded, picture); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

// The revision the screen sends is the one it read from the listing.
func TestALocalThumbnailIsMadeForTheRevisionTheListingShowed(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "a.png"), thumbnailTestPNG(t), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "b.svg"), []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), 0o600); err != nil {
		t.Fatal(err)
	}
	service := &sshcSFTP.Service{}
	engine, credentials := sftpHTTPEngine(t, SFTPHandlers{Service: service, Thumbnails: sshcSFTP.NewThumbnailer(*service)})
	folder := url.QueryEscape(filepath.ToSlash(directory))
	listed := sendKeyRequest(t, engine, credentials, http.MethodGet, "/api/v1/sftp/local/entries?path="+folder, nil, "")
	if listed.Code != http.StatusOK {
		t.Fatalf("list = %d: %s", listed.Code, listed.Body)
	}
	var listing struct {
		Entries []struct {
			Name     string `json:"name"`
			Path     string `json:"path"`
			Revision string `json:"revision"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	revisions := map[string]string{}
	paths := map[string]string{}
	for _, entry := range listing.Entries {
		revisions[entry.Name] = entry.Revision
		paths[entry.Name] = entry.Path
	}

	ask := func(name, revision string) (int, string) {
		endpoint := "/api/v1/sftp/local/thumbnail?path=" + url.QueryEscape(paths[name]) + "&revision=" + url.QueryEscape(revision)
		response := sendKeyRequest(t, engine, credentials, http.MethodGet, endpoint, nil, "")
		return response.Code, response.Header().Get("Content-Type")
	}
	if status, contentType := ask("a.png", revisions["a.png"]); status != http.StatusOK || contentType != "image/jpeg" {
		t.Fatalf("thumbnail = %d %q", status, contentType)
	}
	if status, _ := ask("a.png", "stale"); status != http.StatusConflict {
		t.Fatalf("stale revision = %d", status)
	}
	if status, _ := ask("a.png", ""); status != http.StatusBadRequest {
		t.Fatalf("no revision = %d", status)
	}
	if status, _ := ask("b.svg", revisions["b.svg"]); status != http.StatusUnsupportedMediaType {
		t.Fatalf("svg = %d", status)
	}
}

// poolableContentRemote answers the pool's question without a request to the server.
type poolableContentRemote struct{ *httpContentRemote }

func (poolableContentRemote) Alive() bool { return true }

func TestARemoteThumbnailBorrowsThePanesConnectionAndNeverOpensAnother(t *testing.T) {
	remote := poolableContentRemote{&httpContentRemote{contents: map[string]string{"/work/a.png": string(thumbnailTestPNG(t))}}}
	dials := 0
	pool := sshcSFTP.NewRemotePool(func(context.Context, string) (sshcSFTP.RemoteTarget, error) {
		return sshcSFTP.RemoteTarget{Identity: "edge", Open: func(context.Context) (sshcSFTP.Remote, error) {
			dials++
			return remote, nil
		}}, nil
	})
	service := &sshcSFTP.Service{Open: pool.Open, Borrow: pool.Borrow}
	engine, credentials := sftpHTTPEngine(t, SFTPHandlers{Service: service, Thumbnails: sshcSFTP.NewThumbnailer(*service)})
	listed, err := service.Stat(context.Background(), "edge", "/work/a.png")
	if err != nil {
		t.Fatal(err)
	}
	endpoint := "/api/v1/sftp/edge/thumbnail?path=" + url.QueryEscape("/work/a.png") + "&revision=" + url.QueryEscape(listed.Revision)
	for range 2 {
		response := sendKeyRequest(t, engine, credentials, http.MethodGet, endpoint, nil, "")
		if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "image/jpeg" {
			t.Fatalf("thumbnail = %d %q: %s", response.Code, response.Header().Get("Content-Type"), response.Body)
		}
	}
	if dials != 1 {
		t.Fatalf("dialled %d times", dials)
	}
	if remote.reads != 1 {
		t.Fatalf("read the file %d times", remote.reads)
	}
}
