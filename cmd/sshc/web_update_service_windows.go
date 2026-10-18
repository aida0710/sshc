//go:build windows

package main

import (
	"context"
	"os"
)

func webUpdateServiceStarter() func(context.Context, string) (bool, error) {
	return func(ctx context.Context, executable string) (bool, error) {
		home, err := os.UserHomeDir()
		if err != nil {
			return false, err
		}
		manager, err := newPlatformServiceManager(home)
		if err != nil {
			return false, err
		}
		// The old web engine runs from a renamed backup. Its confirmed handoff
		// owns shutdown; only the new image is checked against the task action.
		return manager.(*taskServiceManager).startIfEnabled(ctx, executable)
	}
}
