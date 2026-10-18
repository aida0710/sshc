//go:build windows

package main

import (
	"context"
	"os"
)

func restartManagedServiceAfterUpdate(ctx context.Context, executable string) (bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, err
	}
	manager, err := newPlatformServiceManager(home)
	if err != nil {
		return false, err
	}
	outdated, err := manager.IsDefinitionOutdated()
	if err != nil {
		return false, err
	}
	if outdated {
		return false, errOutdatedServiceDefinition
	}
	return manager.RestartIfActive(ctx, executable)
}
