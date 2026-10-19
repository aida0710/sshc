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
