//go:build linux

package main

import (
	"os"

	"sshc/internal/keys"
	"sshc/internal/platform/linux"
)

func newPlatformParts() platformParts {
	return platformParts{
		Toolchain: linux.NewToolchain(),
		KeyAgent:  keys.NewAgent(os.LookupEnv),
	}
}
