package main

import "sshc/internal/buildinfo"

func runningBuildInfo(stamp buildinfo.Stamp) *buildinfo.Info {
	buildInfo := buildinfo.Read(stamp)
	found, err := detectInstallation(buildInfo.Executable)
	if err != nil {
		return &buildInfo
	}
	buildInfo.Executable = found.executable
	switch found.manager {
	case managerHomebrew:
		buildInfo.Manager = "homebrew"
	case managerShell, managerPowerShell:
		buildInfo.Manager = found.manager.installerName()
	}
	return &buildInfo
}
