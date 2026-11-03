//go:build windows

package sshclient

import (
	"errors"

	"golang.org/x/sys/windows"
)

// Windows の接続は Winsock のエラーを返す。syscall.ECONNREFUSED などとは一致しない。
func connectionRefused(err error) bool { return errors.Is(err, windows.WSAECONNREFUSED) }

func connectionTimedOut(err error) bool { return errors.Is(err, windows.WSAETIMEDOUT) }

// networkUnreachable は、このマシンに行き先へのルートが無いことを表す。回線が切れたときに返る。
func networkUnreachable(err error) bool {
	return errors.Is(err, windows.WSAENETUNREACH) || errors.Is(err, windows.WSAENETDOWN)
}

// hostUnreachable は、同じネットワークの中で相手が応答しないことを表す。回線はある。
func hostUnreachable(err error) bool { return errors.Is(err, windows.WSAEHOSTUNREACH) }

// connectionReset は、確立した接続が切られたことを表す。
func connectionReset(err error) bool {
	return errors.Is(err, windows.WSAECONNRESET) || errors.Is(err, windows.WSAECONNABORTED)
}
