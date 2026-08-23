//go:build windows

package diagnostics

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// connectionRefused は Unix 互換値と Winsock 固有値の両方を含む。
var connectionRefused = []error{syscall.ECONNREFUSED, windows.WSAECONNREFUSED}
