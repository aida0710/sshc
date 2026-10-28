//go:build !windows

package main

import "context"

// watchSessionEnd は、Windows だけで要る（session_end_windows.go）。Unix の裏のエンジンは、
// 終了のときに SIGTERM を受け取る。
func watchSessionEnd(ctx context.Context) (context.Context, func()) {
	return ctx, func() {}
}
