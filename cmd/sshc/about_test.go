package main

import (
	"testing"
	"time"

	"sshc/internal/buildinfo"
)

// A test binary, like Homebrew's build from the source archive, has no vcs.* settings.
func TestRunningBuildInfoTakesTheCommitFromTheStampWhenGoRecordedNone(t *testing.T) {
	stamp := buildinfo.Stamp{Version: "v1.2.0", Revision: "0123456789abcdef0123456789abcdef01234567", CommitTime: "2026-10-19T08:20:00Z"}
	info := runningBuildInfo(stamp)
	if info.Version != stamp.Version || info.Revision != stamp.Revision || info.CommitTime == nil || info.CommitTime.Format(time.RFC3339) != stamp.CommitTime {
		t.Fatalf("info = %+v", info)
	}
	if info.Modified != nil {
		t.Fatalf("a stamped build reported uncommitted changes: %v", *info.Modified)
	}
}
