package terminal

import "golang.org/x/sys/unix"

const (
	getTermiosRequest = unix.TIOCGETA
	setTermiosRequest = unix.TIOCSETA
)
