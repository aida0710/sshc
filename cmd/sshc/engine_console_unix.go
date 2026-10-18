//go:build !windows

package main

import (
	"os"

	"golang.org/x/sys/unix"
)

func releaseEngineConsole() error { return nil }

func redirectEngineOutput(output *os.File) (func(), error) {
	stdout, err := unix.Dup(1)
	if err != nil {
		return nil, err
	}
	stderr, err := unix.Dup(2)
	if err != nil {
		unix.Close(stdout)
		return nil, err
	}
	restore := func() {
		_ = unix.Dup2(stdout, 1)
		_ = unix.Dup2(stderr, 2)
		_ = unix.Close(stdout)
		_ = unix.Close(stderr)
	}
	if err := unix.Dup2(int(output.Fd()), 1); err != nil {
		restore()
		return nil, err
	}
	if err := unix.Dup2(int(output.Fd()), 2); err != nil {
		restore()
		return nil, err
	}
	restoreGo := replaceGoEngineOutput(output)
	return func() { restoreGo(); restore() }, nil
}
