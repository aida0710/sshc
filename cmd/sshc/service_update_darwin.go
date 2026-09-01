//go:build darwin

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"sshc/internal/storage"
)

func restartManagedServiceAfterUpdate(ctx context.Context, executable string) (bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, fmt.Errorf("resolve home directory: %w", err)
	}
	if !filepath.IsAbs(home) {
		return false, nil
	}
	cleanHome := filepath.Clean(home)
	manager := &launchdServiceManager{
		home:      cleanHome,
		uid:       os.Getuid(),
		files:     storage.OSFileSystem{},
		waitReady: waitForLaunchdServiceReady,
		lock:      launchdOperationLock(cleanHome),
	}
	matches, err := manager.plistMatches(executable)
	if err != nil || !matches {
		return false, err
	}
	launchctl, err := resolveLaunchctl(defaultLaunchctlCandidates, exec.LookPath, os.Stat)
	if err != nil {
		return false, err
	}
	manager.runner = osLaunchdCommandRunner{path: launchctl}
	return manager.RestartIfActive(ctx, executable)
}
