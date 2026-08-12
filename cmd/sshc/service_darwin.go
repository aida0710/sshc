//go:build darwin

package main

import (
	"sshc/internal/platform/macos"
	"sshc/internal/platform/process"
)

// newServiceLoginItem はWebサーバーの組み立てを経由しない。保守コマンドが必要とする
// のはlaunchdだけであり、ブラウザ、SSH toolchain、key agentは作らない。
func newServiceLoginItem(home string) (serviceLoginItem, error) {
	return macos.LoginItem{Runner: process.NewOutputRunner(), Home: home}, nil
}
