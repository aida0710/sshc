//go:build darwin

package main

import (
	"os"

	"sshc/internal/platform/macos"
	"sshc/internal/platform/process"
)

func newPlatformParts(home string) platformParts {
	// OpenSSH のプログラムを起動するすべてのサブシステムが、ひとつのプロセス
	// ランナーとひとつのツールチェーンを共有する。これにより argv、子プロセスの
	// 環境、出力の上限を決める場所はひとつだけになる。
	runner := process.NewOutputRunner()
	toolchain := macos.NewToolchain()
	return platformParts{
		Runner:    runner,
		Toolchain: toolchain,
		Browser:   macos.NewBrowser(runner),
		KeyAgent:  process.NewKeyAgent(runner, toolchain, os.LookupEnv),
		Terminal:  macos.NewTerminal(runner, home),
		LoginItem: macos.LoginItem{Runner: runner, Home: home},
	}
}
