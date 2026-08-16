//go:build windows

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// notifySignals は、この OS で終了を意味するものを理由付きで運ぶ。
//
// **`syscall.SIGBREAK` は書かない。** Windows の Go にその定数は無く、書けば
// コンパイルが通らない。Ctrl-Break を別に登録する必要も無い——ランタイムが
// `CTRL_BREAK_EVENT` を `os.Interrupt` として配るので、Ctrl-C と同じく 130 で
// 終わる。
//
// `syscall.SIGTERM` は、Windows では窓を閉じる・サインアウトする・シャットダウン
// することの写像である。Unix と同じく、監督者が止めたのだから 0 で終わる。
func notifySignals(ctx context.Context) (context.Context, func()) {
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	return watchSignals(ctx, signals, func() { signal.Stop(signals) })
}
