// Package buildinfo describes the running executable, independently of release checks.
package buildinfo

import (
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"time"
)

type Info struct {
	Version    string
	Revision   string
	CommitTime *time.Time
	Modified   *bool
	GoVersion  string
	OS         string
	Arch       string
	Manager    string
	Executable string
}

// Stamp is what the build passes with -ldflags -X. Homebrew builds the tagged
// source archive, which has no .git, so Go cannot record vcs.revision and vcs.time
// there; the formula passes the tagged commit instead.
type Stamp struct {
	Version  string
	Revision string
	// CommitTime is in RFC 3339, like vcs.time.
	CommitTime string
}

func Read(stamp Stamp) Info {
	build, _ := debug.ReadBuildInfo()
	buildInfo := fromBuildInfo(build)
	buildInfo.Version = stamp.Version
	stamp.fillMissing(&buildInfo)
	buildInfo.OS, buildInfo.Arch = runtime.GOOS, runtime.GOARCH
	buildInfo.Manager = "unknown"
	buildInfo.Executable, _ = os.Executable()
	return buildInfo
}

func fromBuildInfo(build *debug.BuildInfo) Info {
	buildInfo := Info{GoVersion: runtime.Version()}
	if build == nil {
		return buildInfo
	}
	if build.GoVersion != "" {
		buildInfo.GoVersion = build.GoVersion
	}
	for _, setting := range build.Settings {
		switch setting.Key {
		case "vcs.revision":
			buildInfo.Revision = setting.Value
		case "vcs.time":
			if timestamp, err := time.Parse(time.RFC3339, setting.Value); err == nil {
				buildInfo.CommitTime = &timestamp
			}
		case "vcs.modified":
			if modified, err := strconv.ParseBool(setting.Value); err == nil {
				buildInfo.Modified = &modified
			}
		}
	}
	return buildInfo
}

// fillMissing uses the stamp only where Go recorded no version control
// information, so a build from a git checkout (brew install --HEAD, make build)
// keeps what Go recorded.
func (stamp Stamp) fillMissing(buildInfo *Info) {
	if buildInfo.Revision == "" {
		buildInfo.Revision = stamp.Revision
	}
	if buildInfo.CommitTime == nil {
		if timestamp, err := time.Parse(time.RFC3339, stamp.CommitTime); err == nil {
			buildInfo.CommitTime = &timestamp
		}
	}
}
