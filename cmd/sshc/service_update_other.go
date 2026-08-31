//go:build !linux

package main

import "context"

func restartManagedServiceAfterUpdate(context.Context) (bool, error) {
	return false, nil
}
