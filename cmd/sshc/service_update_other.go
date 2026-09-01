//go:build !linux && !darwin

package main

import "context"

func restartManagedServiceAfterUpdate(context.Context, string) (bool, error) {
	return false, nil
}
