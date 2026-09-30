//go:build windows

package process

import (
	"os/exec"
	"time"
)

// KillGroupOnCancel は、Windows ではプロセスグループで止めないので、既定どおり command
// だけを止め、出力のパイプが閉じるのを待つ上限を waitDelay にする。
func KillGroupOnCancel(command *exec.Cmd, waitDelay time.Duration) {
	command.WaitDelay = waitDelay
}
