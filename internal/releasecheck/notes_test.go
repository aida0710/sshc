package releasecheck

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"
)

type releaseTransport func(*http.Request) (*http.Response, error)

func (transport releaseTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestNotesIncludeOnlyStableReleasesFromCurrentThroughLatest(t *testing.T) {
	releases := []githubRelease{
		{TagName: "v1.0.0"}, {TagName: "v1.1.0"}, {TagName: "v1.2.0"}, {TagName: "v1.3.0"},
		{TagName: "v1.1.1", Draft: true}, {TagName: "v1.1.2", Prerelease: true},
		{TagName: "v1.2.0-rc.1"}, {TagName: "invalid"}, {TagName: "v1.2.0"},
	}
	notes := selectNotes(releases, "1.1.0", "v1.2.0")
	if len(notes) != 2 || notes[0].Version != "v1.2.0" || notes[1].Version != "v1.1.0" {
		t.Fatalf("notes=%+v", notes)
	}
	if notes := selectNotes(releases, "dev", "v1.2.0"); len(notes) != 3 {
		t.Fatalf("dev notes=%+v", notes)
	}
	if notes := selectNotes(releases, "v2.0.0", "v1.2.0"); len(notes) != 0 {
		t.Fatalf("future notes=%+v", notes)
	}
}

func TestReleaseBodiesAreBoundedWithoutSplittingUTF8(t *testing.T) {
	release := (githubRelease{Body: strings.Repeat("あ", MaxReleaseBodyBytes)}).release()
	if len(release.Body) > MaxReleaseBodyBytes || !utf8.ValidString(release.Body) || len(release.Body) < MaxReleaseBodyBytes-utf8.UTFMax {
		t.Fatalf("body bytes=%d", len(release.Body))
	}
}

func TestCatalogFetchesBoundedNotesWithoutAuthentication(t *testing.T) {
	var paths []string
	checker := Checker{API: "https://api.github.com/repos/aida0710/sshc/releases/latest", HTTP: &http.Client{Transport: releaseTransport(func(request *http.Request) (*http.Response, error) {
		paths = append(paths, request.URL.RequestURI())
		if request.Header.Get("Authorization") != "" {
			t.Fatal("sent authentication")
		}
		body := `{"tag_name":"v1.2.0","published_at":"2026-10-19T00:00:00Z"}`
		if request.URL.Path == "/repos/aida0710/sshc/releases" {
			body = `[{"tag_name":"v1.2.0","body":"<b>plain text</b>","html_url":"https://github.com/aida0710/sshc/releases/tag/v1.2.0"},{"tag_name":"v1.1.0"}]`
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}}
	catalog, err := checker.Catalog(context.Background(), "v1.1.0")
	if err != nil || len(paths) != 2 || paths[1] != "/repos/aida0710/sshc/releases?per_page=100" || len(catalog.Releases) != 2 || catalog.Latest.PublishedAt.IsZero() || catalog.Releases[0].Body != "<b>plain text</b>" {
		t.Fatalf("catalog=%+v paths=%v err=%v", catalog, paths, err)
	}
}
