//go:build !windows

package main

import (
	"os"

	"golang.org/x/sys/unix"
)

func isDetachedTestProcess() bool {
	session, err := unix.Getsid(0)
	return err == nil && session == os.Getpid()
}
