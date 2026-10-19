package releasecheck

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/mod/semver"
)

// MaxReleases bounds the history retained for one engine, including old installations.
const MaxReleases = 100

// MaxReleaseBodyBytes keeps a single release note within a readable, bounded payload.
const MaxReleaseBodyBytes = 64 << 10

// maxReleaseListBytes bounds the GitHub response before decoding its JSON envelope.
const maxReleaseListBytes = 8 << 20

// Catalog contains the latest release and the notes between the running and latest versions.
type Catalog struct {
	Latest   Release
	Releases []Release
}

func (r githubRelease) release() Release {
	body := r.Body
	if len(body) > MaxReleaseBodyBytes {
		body = body[:MaxReleaseBodyBytes]
		for !utf8.ValidString(body) {
			body = body[:len(body)-1]
		}
	}
	return Release{Version: r.TagName, PageURL: r.HTMLURL, PublishedAt: r.PublishedAt, Body: body}
}

func (c Checker) Catalog(ctx context.Context, current string) (Catalog, error) {
	latest, err := c.Latest(ctx)
	if err != nil {
		return Catalog{}, err
	}
	releases, err := c.notes(ctx, current, latest.Version)
	return Catalog{Latest: latest, Releases: releases}, err
}

func (c Checker) notes(ctx context.Context, current, latest string) ([]Release, error) {
	endpoint := strings.TrimSuffix(c.API, "/latest")
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	query := parsed.Query()
	query.Set("per_page", strconv.Itoa(MaxReleases))
	parsed.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	response, err := c.client().Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the releases API answered %d", response.StatusCode)
	}
	var releases []githubRelease
	if err := json.NewDecoder(io.LimitReader(response.Body, maxReleaseListBytes)).Decode(&releases); err != nil {
		return nil, err
	}
	if len(releases) > MaxReleases {
		releases = releases[:MaxReleases]
	}
	return selectNotes(releases, current, latest), nil
}

func selectNotes(releases []githubRelease, current, latest string) []Release {
	selected := make([]Release, 0, len(releases))
	seen := make(map[string]bool)
	currentVersion, currentOK := semanticVersion(current)
	latestVersion, _ := semanticVersion(latest)
	for _, release := range releases {
		version, stable := StableTag(release.TagName)
		if !stable || release.Draft || release.Prerelease || seen[version] {
			continue
		}
		if semver.Compare(version, latestVersion) > 0 || currentOK && semver.Compare(version, currentVersion) < 0 {
			continue
		}
		seen[version] = true
		selected = append(selected, release.release())
	}
	sort.Slice(selected, func(i, j int) bool { return Newer(selected[j].Version, selected[i].Version) })
	return selected
}
