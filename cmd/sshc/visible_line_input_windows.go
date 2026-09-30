//go:build windows

package main

import (
	"context"
	"os"
	"time"
)

// consoleLateCancelGrace は、行入力の ReadConsole が Ctrl-C で何も読まずに戻ったあと、
// Ctrl-C を受けた signal の処理が ctx を取り消すまで待つ上限である。コンソールは
// 読み取りを戻すのと、Ctrl-C を別のスレッドの handler へ届けるのを並んで行うので、
// 読み取りが先に戻ることがある。本当の EOF（Ctrl-Z、閉じた入力）では、この時間だけ
// 遅れて EOF として扱う。利用者が気づかない長さにする。
const consoleLateCancelGrace = 250 * time.Millisecond

// windowsVisibleInputWait は読む前に取り消しだけを確かめる。行入力のコンソールは
// Ctrl-C を受けると ReadConsole を戻すので、コンソールハンドルを待つ必要がない
// （入力イベントが届いても行が揃うまでは読めないので、待っても役に立たない）。
type windowsVisibleInputWait struct {
	ctx context.Context
}

func startVisibleInputWait(ctx context.Context, _ *os.File) (visibleInputWait, error) {
	return windowsVisibleInputWait{ctx: ctx}, nil
}

func (wait windowsVisibleInputWait) waitReadable() error {
	return wait.ctx.Err()
}

// waitForLateCancel は、Ctrl-C で空のまま戻った読み取りを EOF と取り違えないよう、
// ctx の取り消しを consoleLateCancelGrace まで待つ。
func (wait windowsVisibleInputWait) waitForLateCancel() {
	timer := time.NewTimer(consoleLateCancelGrace)
	defer timer.Stop()
	select {
	case <-wait.ctx.Done():
	case <-timer.C:
	}
}

func (windowsVisibleInputWait) close() {}
