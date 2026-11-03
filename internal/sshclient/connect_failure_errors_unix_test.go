//go:build !windows

package sshclient_test

import "syscall"

// この OS の接続が返すエラー番号である。
const (
	errTimedOut           = syscall.ETIMEDOUT
	errNetworkUnreachable = syscall.ENETUNREACH
	errNetworkDown        = syscall.ENETDOWN
	errHostUnreachable    = syscall.EHOSTUNREACH
	errConnectionReset    = syscall.ECONNRESET
)
