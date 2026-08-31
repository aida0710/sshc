//go:build linux

package main

import (
	"context"
	"fmt"
	"os"
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
		home:   filepath.Clean(home),
		runner: osServiceCommandRunner{path: defaultSystemctl},
		files:  storage.OSFileSystem{},
	}
	return manager.RestartIfActive(ctx, executable)
}
