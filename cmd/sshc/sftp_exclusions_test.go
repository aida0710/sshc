package main

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"sshc/internal/api"
)

func TestRecursiveCLIGetReportsExcludedDirectoriesWithoutListingThem(t *testing.T) {
	var calls atomic.Int32
	engine := listingEngine(t, map[string][]api.SFTPEntry{
		"/": {{Name: "data", Path: "/data", Type: api.Directory}},
		"/data": {
			{Name: ".git", Path: "/data/.git", Type: api.Directory},
			{Name: "debug.log", Path: "/data/debug.log", Type: api.File, Size: 100},
			{Name: "keep.txt", Path: "/data/keep.txt", Type: api.File, Size: 4},
		},
	}, &calls)
	plan, err := buildSFTPGetPlan(t.Context(), engine, sftpInvocation{Action: sftpGet, Alias: "edge", Source: "/data", Destination: t.TempDir(), Recursive: true, ExcludePatterns: []string{".git", "*.log"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Files) != 1 || plan.Files[0].Source != "/data/keep.txt" || plan.Bytes != 4 || plan.Skipped != 2 || calls.Load() != 2 {
		t.Fatalf("plan = %+v, listing calls = %d", plan, calls.Load())
	}
	for _, skipped := range plan.SkippedPaths {
		if skipped.Reason != "matched a transfer exclusion rule" {
			t.Fatalf("skipped = %+v", skipped)
		}
	}
}

func TestRecursiveCLIPutExcludesFoldersBeforeReadingOrCheckingTheirDestinations(t *testing.T) {
	source := t.TempDir()
	if err := os.Mkdir(filepath.Join(source, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string]string{".git/config": "skip", "debug.log": "skip", "keep.txt": "keep"} {
		if err := os.WriteFile(filepath.Join(source, filepath.FromSlash(name)), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var calls atomic.Int32
	engine := listingEngine(t, map[string][]api.SFTPEntry{
		"/":       {{Name: "remote", Path: "/remote", Type: api.Directory}},
		"/remote": {},
	}, &calls)
	plan, err := buildSFTPPutPlan(t.Context(), engine, sftpInvocation{Action: sftpPut, Alias: "edge", Source: source, Destination: "/remote/upload", Recursive: true, ExcludePatterns: []string{".git", "*.log"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Files) != 1 || plan.Files[0].Destination != "/remote/upload/keep.txt" || plan.Skipped != 2 || len(plan.Directories) != 1 {
		t.Fatalf("plan = %+v", plan)
	}
}
