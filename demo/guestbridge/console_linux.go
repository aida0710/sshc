package main

import (
	"os"

	"golang.org/x/sys/unix"
)

func resizeConsole(cols, rows uint16) {
	if cols == 0 || rows == 0 {
		return
	}
	console, err := os.OpenFile("/dev/ttyS0", os.O_RDWR, 0)
	if err != nil {
		return
	}
	defer console.Close()
	_ = unix.IoctlSetWinsize(int(console.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Col: cols, Row: rows})
}
