//go:build !windows

package main

import (
	"os/exec"
	"time"

	"sshc/internal/platform/process"
)

// updateCommandWaitDelay は、installer を止めた後に Wait が戻るのを待つ上限で
// ある（exec.Cmd.WaitDelay）。止めても終わらない子や、出力を握ったまま残った子が
// いても、sshc update がそこで止まり続けない。
const updateCommandWaitDelay = 2 * time.Second

// configureUpdateCommand はinstallerが起動したcurl/wgetも同じprocess groupに入れる。
// Ctrl-Cやcontext timeoutでshellだけを止め、downloadだけを残さない。
func configureUpdateCommand(command *exec.Cmd) {
	process.KillGroupOnCancel(command, updateCommandWaitDelay)
}
