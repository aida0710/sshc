//go:build !windows

package process_test

import (
	"bytes"
	"context"
	"os/exec"
	"testing"
	"time"

	"sshc/internal/platform/process"
)

// キャンセルすると、子プロセスが出力のパイプを持っていても、待たずに戻る。
func TestCancellingStopsTheChildrenThatHoldTheOutput(t *testing.T) {
	const generousWaitDelay = 30 * time.Second
	const promptly = 5 * time.Second
	ctx, cancel := context.WithCancel(context.Background())
	// sh が起動した sleep が、標準出力のパイプを持ち続ける。
	command := exec.CommandContext(ctx, "sh", "-c", "sleep 60 & sleep 60")
	var output bytes.Buffer
	command.Stdout = &output
	process.KillGroupOnCancel(command, generousWaitDelay)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	cancel()
	_ = command.Wait()

	if took := time.Since(started); took > promptly {
		t.Fatalf("キャンセルから戻るまで %v かかった（上限 %v）", took, promptly)
	}
}
