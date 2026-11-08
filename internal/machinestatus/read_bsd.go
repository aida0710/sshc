//go:build freebsd || openbsd

package machinestatus

import (
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// readPlatform は sysctl から、起動してからの時間、全体のメモリ、CPU の時間を読む。使用中の
// メモリは OS ごとに数え方が違うので持たない。
func readPlatform() Reading {
	var reading Reading
	if boot, err := unix.SysctlTimeval("kern.boottime"); err == nil {
		reading.Uptime = time.Since(time.Unix(boot.Unix()))
	}
	if total, err := unix.SysctlUint64("hw.physmem"); err == nil {
		reading.MemoryTotal = total
	}
	if raw, err := unix.SysctlRaw("kern.cp_time"); err == nil {
		// long は、このマシンのポインタと同じ大きさである。
		if times, ok := parseCPTime(raw, int(unsafe.Sizeof(uintptr(0)))); ok {
			reading.CPU = &times
		}
	}
	return reading
}
