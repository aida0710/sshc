//go:build freebsd || openbsd

package main

import (
	"os"

	"sshc/internal/keys"
	"sshc/internal/platform/bsd"
)

func newPlatformParts() platformParts {
	return platformParts{
		Toolchain: bsd.NewToolchain(),
		KeyAgent:  keys.NewAgent(os.LookupEnv),
	}
}
