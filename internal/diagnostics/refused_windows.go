//go:build windows

package diagnostics

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// connectionRefused は、この OS が「相手がその港で待っていない」と言うときの errno。
//
// **Winsock の errno は syscall.ECONNREFUSED ではない。** Windows の net が返すのは
// WSAECONNREFUSED であり、両者は別の値である。片方だけを見ていると、拒否された接続が
// 「原因不明の失敗」として報告され、利用者は落ちているホストと閉じている港を
// 区別できない。
var connectionRefused = []error{syscall.ECONNREFUSED, windows.WSAECONNREFUSED}
