package buildinfo

import (
	"runtime/debug"
	"testing"
	"time"
)

func TestReadsVCSSettingsFromBuildInfo(t *testing.T) {
	info := fromBuildInfo(&debug.BuildInfo{GoVersion: "go1.26.9", Settings: []debug.BuildSetting{
		{Key: "vcs.revision", Value: "abcdef1234567890"},
		{Key: "vcs.time", Value: "2026-10-19T01:02:03Z"},
		{Key: "vcs.modified", Value: "false"},
	}})
	if info.Revision != "abcdef1234567890" || info.GoVersion != "go1.26.9" || info.CommitTime == nil || info.CommitTime.Format(time.RFC3339) != "2026-10-19T01:02:03Z" || info.Modified == nil || *info.Modified {
		t.Fatalf("info=%+v", info)
	}
}

func TestMissingOrInvalidVCSInformationRemainsUnknown(t *testing.T) {
	for _, build := range []*debug.BuildInfo{nil, {}, {Settings: []debug.BuildSetting{{Key: "vcs.time", Value: "invalid"}, {Key: "vcs.modified", Value: "invalid"}}}} {
		info := fromBuildInfo(build)
		if info.Revision != "" || info.CommitTime != nil || info.Modified != nil || info.GoVersion == "" {
			t.Fatalf("info=%+v", info)
		}
	}
}

func TestStampFillsOnlyWhatGoDidNotRecord(t *testing.T) {
	stamp := Stamp{Version: "v1.2.0", Revision: "0123456789abcdef0123456789abcdef01234567", CommitTime: "2026-10-19T08:20:00Z"}

	homebrew := fromBuildInfo(&debug.BuildInfo{})
	stamp.fillMissing(&homebrew)
	if homebrew.Revision != stamp.Revision || homebrew.CommitTime == nil || homebrew.CommitTime.Format(time.RFC3339) != stamp.CommitTime || homebrew.Modified != nil {
		t.Fatalf("a build without .git = %+v", homebrew)
	}

	checkout := fromBuildInfo(&debug.BuildInfo{Settings: []debug.BuildSetting{
		{Key: "vcs.revision", Value: "fedcba9876543210"},
		{Key: "vcs.time", Value: "2026-10-20T00:00:00Z"},
	}})
	stamp.fillMissing(&checkout)
	if checkout.Revision != "fedcba9876543210" || checkout.CommitTime.Format(time.RFC3339) != "2026-10-20T00:00:00Z" {
		t.Fatalf("a build from a checkout = %+v", checkout)
	}

	unstamped := fromBuildInfo(&debug.BuildInfo{})
	Stamp{CommitTime: "invalid"}.fillMissing(&unstamped)
	if unstamped.Revision != "" || unstamped.CommitTime != nil {
		t.Fatalf("an empty or invalid stamp = %+v", unstamped)
	}
}
