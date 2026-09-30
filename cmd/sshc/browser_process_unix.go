//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// configureBrowserLauncher は、ブラウザを開く子を新しい session で起動する。
//
// 呼び出し側の process group のままだと、前面で動く `sshc engine` を Ctrl+C で
// 止めたり、ターミナルを閉じたりしたときの SIGINT・SIGHUP が、xdg-open から直接
// 起動したブラウザにも届き、利用者がほかのタブで使っているブラウザごと閉じる。
func configureBrowserLauncher(launcher *exec.Cmd) {
	launcher.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
