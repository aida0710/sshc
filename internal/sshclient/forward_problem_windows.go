//go:build windows

package sshclient

import (
	"errors"

	"golang.org/x/sys/windows"
)

// Windows の bind は Winsock のエラーを返す。syscall.EADDRINUSE とは一致しない。
func addressInUse(err error) bool { return errors.Is(err, windows.WSAEADDRINUSE) }

// 予約済みの範囲のポートや、ほかのプロセスが排他で使うポートは WSAEACCES になる。
func listenNotPermitted(err error) bool {
	return errors.Is(err, windows.WSAEACCES) || errors.Is(err, windows.ERROR_ACCESS_DENIED)
}
