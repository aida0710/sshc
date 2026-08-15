package main

import (
	"context"
	"testing"
	"time"

	"sshc/internal/handoff"
)

// **ロックを取れることと、入口が読めることは同時ではない。** 勝った方は
// listener を上げてから handoff を書くので、負けた方がその隙に読むと
// 「sshc: not running」になる。ほぼ同時に打った 2 つのうち片方だけが、
// 理由の無い失敗を受け取ることになっていた。
func TestWaitForHandoffWaitsForTheWinnerToWrite(t *testing.T) {
	stateDir := t.TempDir()
	go func() {
		time.Sleep(150 * time.Millisecond)
		_ = handoff.Write(stateDir, testHandoff("http://127.0.0.1:1"))
	}()

	waitForHandoff(context.Background(), stateDir)
	if _, err := handoff.Read(stateDir); err != nil {
		t.Fatalf("waitForHandoff returned before the handoff was readable: %v", err)
	}
}

// **待つのは待てるあいだだけである。** Ctrl-C を押した人を、居ないかもしれない
// エンジンのために 4 秒立たせない。
func TestWaitForHandoffGivesUpWhenTheContextIsDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		waitForHandoff(ctx, t.TempDir())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("waitForHandoff kept waiting after the context was done")
	}
}
