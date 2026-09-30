//go:build !windows

package sshclient

import (
	"errors"
	"syscall"
)

func addressInUse(err error) bool { return errors.Is(err, syscall.EADDRINUSE) }

func listenNotPermitted(err error) bool {
	return errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM)
}
