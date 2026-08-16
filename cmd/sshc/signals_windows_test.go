//go:build windows

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

const signalHelperEnvironment = "SSHC_SIGNAL_HELPER"

// **Ctrl-Break を別に登録しない。** ランタイムがそれを os.Interrupt として
// 配るので、Ctrl-C と同じ 130 になる。ここはその事実そのものを固定する
// ——`syscall.SIGBREAK` を書こうとした版は、そもそもコンパイルが通らなかった。
func TestWindowsCtrlBreakEndsWithTheInterruptCode(t *testing.T) {
	if os.Getenv(signalHelperEnvironment) != "" {
		return
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	helper := exec.Command(executable, "-test.run=TestWindowsCtrlBreakEndsWithTheInterruptCode")
	helper.Env = append(os.Environ(), signalHelperEnvironment+"=1")
	helper.SysProcAttr = &windows.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
	if err := helper.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = helper.Process.Kill() }()

	// プロセスグループへ Ctrl-Break を送る。**Ctrl-C はグループを指定できない。**
	deadline := time.Now().Add(10 * time.Second)
	for {
		err := windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(helper.Process.Pid))
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Skipf("Ctrl-Break could not be delivered in this environment: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	waited := make(chan error, 1)
	go func() { waited <- helper.Wait() }()
	select {
	case err := <-waited:
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("the helper exited with %v, want a non-zero code", err)
		}
		if code := exitErr.ExitCode(); code != 130 {
			t.Fatalf("Ctrl-Break exit = %d, want 130", code)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the helper did not exit after Ctrl-Break")
	}
}

// notifySignals は、この OS に存在する合図だけを登録する。
func TestWindowsSignalsRegisterAndStopCleanly(t *testing.T) {
	ctx, stop := notifySignals(context.Background())
	stop()
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("stopping the signal watch left its context open")
	}
	if cause := context.Cause(ctx); cause != nil && !errors.Is(cause, context.Canceled) {
		t.Fatalf("cause after a clean stop = %v", cause)
	}
}
