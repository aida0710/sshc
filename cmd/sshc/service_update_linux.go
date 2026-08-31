//go:build linux

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
	// systemdがないLinuxでも、管理unitが存在しない通常のupdateを妨げない。
	// Statusはunitがsshc管理下で動作中と確認した場合だけsystemctlを実行する。
	manager := &linuxServiceManager{
		home:      filepath.Clean(home),
		files:     storage.OSFileSystem{},
		waitReady: waitForServiceReady,
		lock:      serviceOperationLock(filepath.Clean(home)),
	}
	matches, err := manager.unitMatches(executable)
	if err != nil || !matches {
		return false, err
	}
	systemctl, err := resolveSystemctl(defaultSystemctlCandidates, exec.LookPath, os.Stat)
	if err != nil {
		return false, err
	}
	manager.runner = osServiceCommandRunner{path: systemctl}
	return manager.RestartIfActive(ctx, executable)
}
