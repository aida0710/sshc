package terminal

import "golang.org/x/sys/unix"

const (
	getTermiosRequest = unix.TCGETS
	setTermiosRequest = unix.TCSETS
)
