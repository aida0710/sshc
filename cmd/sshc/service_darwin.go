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
	manager := newServiceManagerWithoutTool(filepath.Clean(home))
	if err := manager.resolveTool(); err != nil {
		return nil, err
	}
	return manager, nil
}

// newServiceManagerWithoutTool は、launchctl をまだ探していない manager を返す。
// home は絶対パスで、Clean 済みであること。
func newServiceManagerWithoutTool(home string) *launchdServiceManager {
	return &launchdServiceManager{
		home:      home,
		uid:       os.Getuid(),
		files:     storage.OSFileSystem{},
		waitReady: waitForLaunchdServiceReady,
		lock:      serviceOperationLock(home),
	}
}

// resolveTool は、launchctl を探して、この manager が実行するツールにする。
func (manager *launchdServiceManager) resolveTool() error {
	launchctl, err := resolveLaunchctl(defaultLaunchctlCandidates, exec.LookPath, os.Stat)
	if err != nil {
		return err
	}
	manager.runner = osServiceCommandRunner{path: launchctl}
	return nil
}

func resolveLaunchctl(candidates []string, lookPath func(string) (string, error), stat func(string) (os.FileInfo, error)) (string, error) {
	return resolveServiceTool("launchctl", candidates, lookPath, stat)
}
