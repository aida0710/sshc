//go:build darwin

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"

	"sshc/internal/storage"
)

var defaultLaunchctlCandidates = []string{"/bin/launchctl", "/usr/bin/launchctl"}

func newPlatformServiceManager(home string) (engineServiceManager, error) {
	if !filepath.IsAbs(home) {
		return nil, errors.New("home directory is not absolute")
	}
	launchctl, err := resolveLaunchctl(defaultLaunchctlCandidates, exec.LookPath, os.Stat)
	if err != nil {
		return nil, err
	}
	cleanHome := filepath.Clean(home)
	return &launchdServiceManager{
		home:      cleanHome,
		uid:       os.Getuid(),
		runner:    osServiceCommandRunner{path: launchctl},
		files:     storage.OSFileSystem{},
		waitReady: waitForLaunchdServiceReady,
		lock:      serviceOperationLock(cleanHome),
	}, nil
}

func resolveLaunchctl(candidates []string, lookPath func(string) (string, error), stat func(string) (os.FileInfo, error)) (string, error) {
	return resolveServiceTool("launchctl", candidates, lookPath, stat)
}
