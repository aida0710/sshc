//go:build darwin

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"sshc/internal/enginelock"
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
		runner:    osLaunchdCommandRunner{path: launchctl},
		files:     storage.OSFileSystem{},
		waitReady: waitForLaunchdServiceReady,
		lock:      launchdOperationLock(cleanHome),
	}, nil
}

func launchdOperationLock(home string) func() (func() error, error) {
	path := filepath.Join(home, ".config", "sshc", "service.mutation.lock")
	return func() (func() error, error) {
		release, err := enginelock.Acquire(path)
		if errors.Is(err, enginelock.ErrRunning) {
			return nil, errors.New("another sshc service operation is in progress")
		}
		return release, err
	}
}

func resolveLaunchctl(candidates []string, lookPath func(string) (string, error), stat func(string) (os.FileInfo, error)) (string, error) {
	paths := append([]string(nil), candidates...)
	if found, err := lookPath("launchctl"); err == nil {
		if !filepath.IsAbs(found) {
			found, err = filepath.Abs(found)
			if err != nil {
				return "", fmt.Errorf("resolve launchctl path: %w", err)
			}
		}
		paths = append(paths, filepath.Clean(found))
	}
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if !filepath.IsAbs(path) || containsControl(path) {
			continue
		}
		path = filepath.Clean(path)
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		info, err := stat(path)
		if err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0 {
			return path, nil
		}
	}
	return "", errors.New("cannot find an executable launchctl")
}
