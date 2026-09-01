//go:build !linux && !darwin

package main

import "errors"

func newPlatformServiceManager(string) (engineServiceManager, error) {
	return nil, errors.New("managed user services are supported only on Linux and macOS")
}
