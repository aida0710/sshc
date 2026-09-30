package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
)

// シグナルで止まった理由。context.WithCancelCause でこれを運び、終了コードは
// 呼び出し側が理由から決める（engine は exitForCause、Serial／Telnet は
// transportExitCode）。
//
// errInterrupted は利用者の Ctrl-C で、終了コード 130 に変換される。engine を
// 確かめている間の Ctrl-C も同じ理由で表す。errTerminated は、監督者の SIGTERM か、
// 端末を閉じたときの SIGHUP である。
var (
	errInterrupted = errors.New("interrupted")
	errTerminated  = errors.New("terminated")
)

// watchSignals は、OS 別のシグナル集合をひとつの理由付き context に変える。
func watchSignals(ctx context.Context, signals chan os.Signal, stop func()) (context.Context, func()) {
	signalCtx, cancel := context.WithCancelCause(ctx)
	go func() {
		select {
		case received := <-signals:
			if received == os.Interrupt {
				cancel(errInterrupted)
				return
			}
			cancel(errTerminated)
		case <-signalCtx.Done():
		}
	}()
	return signalCtx, func() {
		stop()
		cancel(nil)
		signal.Stop(signals)
	}
}

// interactiveStopExitCode は、対話の処理（sshc ssh の選択画面と SSH の接続）が
// シグナルで止まっていれば、その終了コードと true を返す。
//
// Ctrl-C は 130。監督者の SIGTERM と、ターミナルを閉じたときの SIGHUP は失敗では
// ないので 0 にする。engine と Serial／Telnet の対話接続と同じ扱いである。
func interactiveStopExitCode(ctx context.Context) (int, bool) {
	if ctx.Err() == nil {
		return 0, false
	}
	if errors.Is(context.Cause(ctx), errTerminated) {
		return 0, true
	}
	return exitInterrupted, true
}
