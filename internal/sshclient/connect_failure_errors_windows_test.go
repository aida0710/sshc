//go:build windows

package sshclient_test

import "golang.org/x/sys/windows"

// この OS の接続が返すエラー番号である。
const (
	errTimedOut           = windows.WSAETIMEDOUT
	errNetworkUnreachable = windows.WSAENETUNREACH
	errNetworkDown        = windows.WSAENETDOWN
	errHostUnreachable    = windows.WSAEHOSTUNREACH
	errConnectionReset    = windows.WSAECONNRESET
)
