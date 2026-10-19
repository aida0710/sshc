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

func Read(version string) Info {
	build, _ := debug.ReadBuildInfo()
	buildInfo := fromBuildInfo(build)
	buildInfo.Version = version
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
