//go:build !windows

package diagnostics

import "syscall"

// connectionRefused は、この OS が「相手がその港で待っていない」と言うときの errno。
var connectionRefused = []error{syscall.ECONNREFUSED}
