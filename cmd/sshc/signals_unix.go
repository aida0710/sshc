//go:build unix

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// notifySignals は、この OS で終了を意味するシグナルを理由付きで運ぶ。
//
// Ctrl-C と SIGTERM は同じ後始末を通るが、終わり方は違う。前者はユーザーが
// 止めたので 130、後者は監督者が止めたので 0 である。
func notifySignals(ctx context.Context) (context.Context, func()) {
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	return watchSignals(ctx, signals, func() { signal.Stop(signals) })
}
