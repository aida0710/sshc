//go:build windows

package integration

import (
	"os/exec"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// **止め方で終わり方が変わる。** Ctrl-Break は人が止めたので 130 である。
// Go のランタイムは `CTRL_BREAK_EVENT` を `os.Interrupt` として配るので、
// Unix の Ctrl-C と同じ番号で終わる——`signals_windows.go` がそう書いている
// ことを、実際に一発送って確かめる。
//
// **Ctrl-C ではなく Ctrl-Break を送る。** `GenerateConsoleCtrlEvent` は
// `CTRL_C_EVENT` をプロセスグループ指定で送れない（0 すなわち自分の
// グループ全体にしか送れない）ので、送った側のテストごと落ちる。Ctrl-Break は
// グループを指定でき、engine から見れば同じ `os.Interrupt` である。
func TestCtrlBreakStopsTheEngineWithTheInterruptedStatus(t *testing.T) {
	home := isolatedHome(t)
	engine := startInOwnGroup(t, home)
	waitForFile(t, handoffPath(home), 30*time.Second, engine)

	if err := windows.GenerateConsoleCtrlEvent(
		windows.CTRL_BREAK_EVENT, uint32(engine.Command.Process.Pid),
	); err != nil {
		t.Fatalf("send Ctrl-Break: %v", err)
	}

	if code := engine.wait(t, 30*time.Second); code != 130 {
		t.Errorf("exit = %d, want 130\n%s", code, engine.Stderr.String())
	}
	// **畳んでから終わる。** 次の owner のために席が空いていることが、
	// 片付けが最後まで走った証拠である。LockFileEx がプロセスと一緒に
	// 消えただけなら、ここは通っても畳んだことにはならない——handoff を
	// 置き換えられることまで見る。
	takeOverAsHeadless(t, home)
}

// startInOwnGroup は、自分のプロセスグループを持つ engine を起こす。
//
// **これが無いと Ctrl-Break を宛てられない。** `GenerateConsoleCtrlEvent` は
// プロセスグループへ送るものであり、グループを持たない子は親のグループに
// 属している——そこへ送れば、送った側のテストも一緒に受け取る。
func startInOwnGroup(t *testing.T, home string) *testProcess {
	t.Helper()
	command := exec.Command(binaryPath, "engine")
	command.Env = []string{
		"PATH=" + osPath(),
		"HOME=" + home,
		"USERPROFILE=" + home,
		"TMPDIR=" + home,
		"SystemRoot=" + osSystemRoot(),
	}
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
	return startPrepared(t, command)
}
