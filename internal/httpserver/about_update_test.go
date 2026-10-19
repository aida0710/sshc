package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"sshc/internal/api"
	"sshc/internal/buildinfo"
	"sshc/internal/releasecheck"
	"sshc/internal/releasewatch"
)

func TestAboutReportsTheRunningBuildAndUnknownVCSFields(t *testing.T) {
	engine := echo.New()
	registerAboutRoute(engine, "v1.0.0", &buildinfo.Info{Version: "v1.0.0", Manager: "install.ps1", OS: "windows", Arch: "arm64", GoVersion: "go1.26.9", Executable: `C:\sshc\sshc.exe`})
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/about", nil))
	var about api.About
	if err := json.Unmarshal(response.Body.Bytes(), &about); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || about.Version != "v1.0.0" || about.Manager != "install.ps1" || about.Modified != nil || about.CommitTime != nil || about.Executable != `C:\sshc\sshc.exe` {
		t.Fatalf("about=%+v status=%d", about, response.Code)
	}
}

func TestUpdateReadsRetainedResultsAndPostRefreshesIncludingFailures(t *testing.T) {
	calls := 0
	watcher := releasewatch.New(context.Background(), func(context.Context) (releasecheck.Catalog, error) {
		calls++
		if calls > 1 {
			return releasecheck.Catalog{}, errors.New("upstream unavailable")
		}
		return releasecheck.Catalog{Latest: releasecheck.Release{Version: "v1.2.0", PageURL: "https://github.com/aida0710/sshc/releases/tag/v1.2.0", PublishedAt: time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)}, Releases: []releasecheck.Release{{Version: "v1.2.0", Body: "<script>text only</script>"}}}, nil
	})
	defer watcher.Stop()
	engine := echo.New()
	registerUpdateRoutes(engine, &UpdateHandlers{Current: "v1.0.0", Watcher: watcher})
	request := func(method, path string) api.UpdateStatus {
		t.Helper()
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(method, path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("%s %s: %d %s", method, path, response.Code, response.Body.String())
		}
		var status api.UpdateStatus
		if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
			t.Fatal(err)
		}
		return status
	}
	first := request(http.MethodGet, "/api/v1/update")
	cached := request(http.MethodGet, "/api/v1/update")
	if calls != 1 || first.CheckedAt == nil || cached.CheckedAt == nil || !first.CheckedAt.Equal(*cached.CheckedAt) || !cached.Available || cached.PublishedAt == nil || cached.Releases == nil || (*cached.Releases)[0].Body != "<script>text only</script>" {
		t.Fatalf("cached=%+v calls=%d", cached, calls)
	}
	failed := request(http.MethodPost, "/api/v1/update/check")
	if calls != 2 || failed.CheckError == nil || *failed.CheckError != "upstream unavailable" || failed.Latest == nil || *failed.Latest != "v1.2.0" {
		t.Fatalf("failed=%+v calls=%d", failed, calls)
	}
	request(http.MethodGet, "/api/v1/update")
	if calls != 2 {
		t.Fatalf("GET retried a failed refresh: %d", calls)
	}
}

func TestManualUpdateCheckRequiresTheExistingBrowserSecurity(t *testing.T) {
	harness := newWebUpdateHarness(t)
	calls := 0
	harness.handlers.Watcher = releasewatch.New(context.Background(), func(context.Context) (releasecheck.Catalog, error) { calls++; return releasecheck.Catalog{}, nil })
	defer harness.handlers.Watcher.Stop()
	unauthenticated := httptest.NewRecorder()
	harness.engine.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodPost, "http://"+keyTestHost+"/api/v1/update/check", nil))
	if unauthenticated.Code == http.StatusOK || calls != 0 {
		t.Fatal("unauthenticated check reached GitHub")
	}
	response := sendKeyRequest(t, harness.engine, harness.credentials, http.MethodPost, "/api/v1/update/check", []byte(`{}`), "")
	if response.Code != http.StatusOK || calls != 1 {
		t.Fatalf("authenticated check: %d %s calls=%d", response.Code, response.Body.String(), calls)
	}
}
