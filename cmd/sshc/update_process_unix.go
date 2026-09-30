//go:build !windows

package main

import (
	"os/exec"
	"time"

	"sshc/internal/platform/process"
)

// updateCommandTerminationGrace は、取消でprocess groupへSIGTERMを送ってから、残ったshellを
// SIGKILLで止めるまでの猶予である。install.shのtrapが置き先の一時ファイルと作業ディレクトリを
// 消すのも、curl/wgetがSIGTERMで終わるのも、この時間で足りる。上限があるので、止めても
// 終わらない子がいても、sshc update がそこで止まり続けない。
const updateCommandTerminationGrace = 2 * time.Second

// configureUpdateCommand はinstallerが起動したcurl/wgetも同じprocess groupに入れる。
// Ctrl-Cやcontext timeoutでは、まずgroupへSIGTERMを送り、install.shのtrapに後始末をさせる。
//
// 戻り値はWaitが戻った後に呼ぶ。SIGTERMを無視してgroupに残ったものをSIGKILLで止め、
// shellだけが終わってdownloadが残ることのないようにする。
func configureUpdateCommand(command *exec.Cmd) (killLeftovers func()) {
	return process.TerminateGroupOnCancel(command, updateCommandTerminationGrace)
}
