//go:build windows

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

const signalHelperEnvironment = "SSHC_SIGNAL_HELPER"

// Ctrl-Break を別に登録しない。ランタイムがそれを os.Interrupt として
// 配るので、Ctrl-C と同じ 130 になる。ここはその事実そのものを固定する
// `syscall.SIGBREAK` を書こうとした版は、そもそもコンパイルが通らなかった。
func TestWindowsCtrlBreakEndsWithTheInterruptCode(t *testing.T) {
	if os.Getenv(signalHelperEnvironment) != "" {
		// 子は本物の登録をしてから待つ。登録しないまま待てば既定の動作で
		// 殺され、終了コードは 130 ではなく NT の状態値になる。最初に書いた
		// 版はまさにそれで、実 Windows がそう教えてくれた。
		ctx, stop := notifySignals(context.Background())
		defer stop()
		// 登録し終えたことを先に告げる。親がそれを待たずに送ると、まだ
		// 登録の前に届いた合図は既定の動作で処理され、終了コードは 130 では
		// なく NT の状態値になる。
		fmt.Println("ready")
		<-ctx.Done()
		os.Exit(exitForCause(context.Cause(ctx), slog.New(slog.NewTextHandler(io.Discard, nil))))
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	helper := exec.Command(executable, "-test.run=TestWindowsCtrlBreakEndsWithTheInterruptCode")
	helper.Env = append(os.Environ(), signalHelperEnvironment+"=1")
	helper.SysProcAttr = &windows.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
	output, err := helper.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := helper.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = helper.Process.Kill() }()

	ready := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(output).ReadString('\n')
		ready <- strings.TrimSpace(line)
	}()
	select {
	case line := <-ready:
		if line != "ready" {
			t.Fatalf("the helper announced %q, want ready", line)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the helper never finished registering for signals")
	}

	// プロセスグループへ Ctrl-Break を送る。Ctrl-C はグループを指定できない。
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
