//go:build windows

package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

var procSendMessageW = user32.NewProc("SendMessageW")

// 終了の知らせを受けたら、エンジンを止める理由を運び、止まり終えるまで知らせから戻らない。
// 戻ると Windows はいつでもプロセスを終わらせてよいので、記録を書く前に戻ってはならない。
func TestTheEndOfAWindowsSessionStopsTheEngineAndWaitsUntilItHasStopped(t *testing.T) {
	watch := startSessionEndWatch(context.Background())
	if watch.window == 0 {
		t.Fatal("the hidden window was not created")
	}
	if answer, _, _ := procSendMessageW.Call(watch.window, windowMessageQueryEndSession, 0, 0); answer != allowSessionEnd {
		t.Fatalf("WM_QUERYENDSESSION answered %d, want %d", answer, allowSessionEnd)
	}
	returned := make(chan struct{})
	go func() {
		procSendMessageW.Call(watch.window, windowMessageEndSession, 1, 0)
		close(returned)
	}()
	select {
	case <-watch.ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the engine was not told to stop")
	}
	if cause := context.Cause(watch.ctx); !errors.Is(cause, errTerminated) {
		t.Fatalf("stop cause = %v, want errTerminated", cause)
	}
	select {
	case <-returned:
		t.Fatal("WM_ENDSESSION returned before the engine had stopped")
	case <-time.After(200 * time.Millisecond):
	}
	watch.finish()
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("WM_ENDSESSION did not return after the engine stopped")
	}
}

// 取りやめになった終了（wParam が 0）では止めない。
func TestACancelledEndOfSessionLeavesTheEngineRunning(t *testing.T) {
	watch := startSessionEndWatch(context.Background())
	defer watch.finish()
	procSendMessageW.Call(watch.window, windowMessageEndSession, 0, 0)
	if watch.ctx.Err() != nil {
		t.Fatalf("a cancelled end of session stopped the engine: %v", context.Cause(watch.ctx))
	}
}
