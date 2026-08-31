//go:build !linux

package main

import "errors"

func newPlatformServiceManager(string) (engineServiceManager, error) {
	return nil, errors.New("systemd user services are supported only on Linux")
}
