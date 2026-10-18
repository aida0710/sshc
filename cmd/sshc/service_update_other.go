//go:build !linux && !darwin && !windows

package main

import "context"

func restartManagedServiceAfterUpdate(context.Context, string) (bool, error) {
	return false, nil
}
