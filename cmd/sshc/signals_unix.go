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
// Ctrl-C と SIGTERM は同じ後始末を通るが、理由は分けて運ぶ。前者はユーザーが、
// 後者は監督者が止めた。端末を閉じたときの SIGHUP も後者と同じ後始末を通す。
// 既定のまま落とすと handoff が残り、次に同じ port を取った process が CLI から
// engine に見える。
func notifySignals(ctx context.Context) (context.Context, func()) {
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	return watchSignals(ctx, signals, func() { signal.Stop(signals) })
}
