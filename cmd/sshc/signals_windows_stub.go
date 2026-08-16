//go:build windows

package main

import (
	"context"
	"os"
	"os/signal"
)

// notifySignals は、Windows では Ctrl-C だけを扱う一時的な実装である。
//
// **Ctrl-Break の対応ではない。** Windows Task 6 がこれを本物へ差し替え、
// ネイティブの Ctrl-C/Ctrl-Break 試験を伴って置き換える。ここに Unix の
// シグナル定数を持ち込まないためだけに分けてある。
func notifySignals(ctx context.Context) (context.Context, func()) {
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt)
	return watchSignals(ctx, signals, func() { signal.Stop(signals) })
}
