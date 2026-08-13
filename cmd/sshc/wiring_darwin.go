//go:build darwin

package main

import (
	"os"

	"sshc/internal/keys"
	"sshc/internal/platform/macos"
	"sshc/internal/platform/process"
)

func newPlatformParts(home string) platformParts {
	// この組み立ての中では、OpenSSH のプログラムを起動するすべてのサブシステムが
	// ひとつのプロセスランナーとひとつのツールチェーンを共有する。これにより argv、
	// 子プロセスの環境、出力の上限を決める場所はひとつだけになる。
	runner := process.NewOutputRunner()
	toolchain := macos.NewToolchain()
	return platformParts{
		Runner:    runner,
		Toolchain: toolchain,
		Browser:   macos.NewBrowser(runner),
		KeyAgent:  keys.NewAgent(os.LookupEnv),
		LoginItem: macos.LoginItem{Runner: runner, Home: home},
	}
}
