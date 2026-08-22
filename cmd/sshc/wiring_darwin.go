//go:build darwin

package main

import (
	"os"

	"sshc/internal/keys"
	"sshc/internal/platform/macos"
)

func newPlatformParts() platformParts {
	return platformParts{
		Toolchain: macos.NewToolchain(),
		KeyAgent:  keys.NewAgent(os.LookupEnv),
	}
}
