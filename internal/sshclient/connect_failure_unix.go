//go:build !windows

package sshclient

import (
	"errors"
	"syscall"
)

func connectionRefused(err error) bool { return errors.Is(err, syscall.ECONNREFUSED) }

func connectionTimedOut(err error) bool { return errors.Is(err, syscall.ETIMEDOUT) }

// networkUnreachable は、このマシンに行き先へのルートが無いことを表す。回線が切れたときに返る。
func networkUnreachable(err error) bool {
	return errors.Is(err, syscall.ENETUNREACH) || errors.Is(err, syscall.ENETDOWN)
}

// hostUnreachable は、同じネットワークの中で相手が応答しないことを表す。回線はある。
func hostUnreachable(err error) bool { return errors.Is(err, syscall.EHOSTUNREACH) }

// connectionReset は、確立した接続が切られたことを表す。スマホで回線が切り替わると
// ECONNABORTED も返る。
func connectionReset(err error) bool {
	return errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNABORTED) || errors.Is(err, syscall.EPIPE)
}
