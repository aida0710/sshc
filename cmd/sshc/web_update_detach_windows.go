//go:build windows

package main

import "os/exec"

// Windows has no automatic web updater; keep the composition root buildable.
func configureDetachedUpdateProcess(*exec.Cmd) {}
