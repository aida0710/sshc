package main

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
)

// listingEngine answers directory listings from a map and counts them.
func listingEngine(t *testing.T, listings map[string][]sftpCLIEntry, calls *atomic.Int32) *engineAPI {
	t.Helper()
	server := engineTestServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", request.Method, request.URL.String())
			response.WriteHeader(http.StatusNotFound)
			return
		}
		calls.Add(1)
		remotePath := request.URL.Query().Get("path")
		entries, exists := listings[remotePath]
		if !exists {
			writeTestJSON(response, http.StatusNotFound, map[string]string{"code": "sftp_not_found", "message": "not found"})
			return
		}
		writeTestJSON(response, http.StatusOK, sftpCLIListing{Path: remotePath, Entries: entries})
	}))
	t.Cleanup(server.Close)
	return testSFTPEngine(server)
}

func TestPutPlanListsEachRemoteDirectoryOnceAndFollowsLocalLinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	source := t.TempDir()
	mustWrite := func(name, contents string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(source, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(source, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.txt", "b.txt", "c.txt", "nested/d.txt"} {
		mustWrite(name, name)
	}
	if err := os.Symlink("a.txt", filepath.Join(source, "link-to-a")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("nested", filepath.Join(source, "link-to-nested")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", filepath.Join(source, "dangling")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("..", filepath.Join(source, "nested", "up")); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	engine := listingEngine(t, map[string][]sftpCLIEntry{
		"/":                {{Name: "remote", Path: "/remote", Type: "directory"}},
		"/remote":          {{Name: "existing", Path: "/remote/existing", Type: "directory"}},
		"/remote/existing": {{Name: "a.txt", Path: "/remote/existing/a.txt", Type: "file", Size: 1}},
	}, &calls)

	plan, err := buildSFTPPutPlan(t.Context(), engine, sftpInvocation{Action: sftpPut, Alias: "edge", Source: source, Destination: "/remote/existing", Recursive: true})
	if err != nil {
		t.Fatalf("buildSFTPPutPlan() = %v", err)
	}
	// One listing per remote directory touched, whatever the number of files:
	// the destination's parent, then the new root, which does not exist yet,
	// so nothing below it is asked about.
	if got := calls.Load(); got != 2 {
		t.Fatalf("listings = %d, want 2 (/remote and the missing root)", got)
	}
	var files []string
	for _, file := range plan.Files {
		files = append(files, file.Destination)
	}
	base := "/remote/existing/" + filepath.Base(source)
	want := []string{base + "/a.txt", base + "/b.txt", base + "/c.txt", base + "/link-to-a", base + "/link-to-nested/d.txt", base + "/nested/d.txt"}
	if len(files) != len(want) {
		t.Fatalf("planned files = %v, want %v", files, want)
	}
	for _, expected := range want {
		found := false
		for _, file := range files {
			found = found || file == expected
		}
		if !found {
			t.Fatalf("planned files = %v, missing %s", files, expected)
		}
	}
	// The dangling link and the two ways back up the tree are left out.
	if plan.Skipped != 3 || len(plan.SkippedPaths) != 3 || plan.SkippedPaths[0].Path != filepath.Join(source, "dangling") ||
		plan.SkippedPaths[1].Path != filepath.Join(source, "link-to-nested", "up") || plan.SkippedPaths[2].Path != filepath.Join(source, "nested", "up") {
		t.Fatalf("skipped = %+v", plan.SkippedPaths)
	}
}

func TestPutPlanNamesAMissingRemoteDirectoryForASingleFile(t *testing.T) {
	source := filepath.Join(t.TempDir(), "one.txt")
	if err := os.WriteFile(source, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	engine := listingEngine(t, map[string][]sftpCLIEntry{"/": {{Name: "remote", Path: "/remote", Type: "directory"}}}, &calls)
	_, err := buildSFTPPutPlan(t.Context(), engine, sftpInvocation{Action: sftpPut, Alias: "edge", Source: source, Destination: "/remote/absent/one.txt"})
	if !errors.Is(err, errSFTPRemoteDirectoryMissing) {
		t.Fatalf("buildSFTPPutPlan() = %v, want %v", err, errSFTPRemoteDirectoryMissing)
	}
}

func TestGetPlanTransfersWhatRemoteLinksPointToAndSkipsBrokenOnes(t *testing.T) {
	destination := t.TempDir()
	var calls atomic.Int32
	engine := listingEngine(t, map[string][]sftpCLIEntry{
		"/": {{Name: "srv", Path: "/srv", Type: "directory"}},
		"/srv": {
			{Name: "notes.txt", Path: "/srv/notes.txt", Type: "file", Size: 5, ModifiedAt: "2019-05-06T07:08:09Z"},
			{Name: "notes-link", Path: "/srv/notes-link", Type: "symlink", Size: 5, ModifiedAt: "2019-05-06T07:08:09Z", LinkTarget: "notes.txt", TargetType: "file"},
			{Name: "data-link", Path: "/srv/data-link", Type: "symlink", Size: 4, LinkTarget: "data", TargetType: "directory"},
			{Name: "broken", Path: "/srv/broken", Type: "symlink", Size: 7, LinkTarget: "missing"},
			{Name: "socket", Path: "/srv/socket", Type: "other", Size: 0},
		},
		"/srv/data-link": {{Name: "inside.txt", Path: "/srv/data-link/inside.txt", Type: "file", Size: 6}},
	}, &calls)

	plan, err := buildSFTPGetPlan(t.Context(), engine, sftpInvocation{Action: sftpGet, Alias: "edge", Source: "/srv", Destination: destination, Recursive: true})
	if err != nil {
		t.Fatalf("buildSFTPGetPlan() = %v", err)
	}
	var files []string
	for _, file := range plan.Files {
		files = append(files, file.Source)
		if file.Source == "/srv/notes-link" && file.ModifiedUnix != 1557126489000 {
			t.Fatalf("link modified = %d, want the file's time", file.ModifiedUnix)
		}
	}
	if len(files) != 3 || files[0] != "/srv/data-link/inside.txt" || files[1] != "/srv/notes-link" || files[2] != "/srv/notes.txt" {
		t.Fatalf("planned files = %v", files)
	}
	if plan.Skipped != 2 || len(plan.SkippedPaths) != 2 || plan.SkippedPaths[0].Path != "/srv/broken" || plan.SkippedPaths[1].Path != "/srv/socket" {
		t.Fatalf("skipped = %+v", plan.SkippedPaths)
	}
}
