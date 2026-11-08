//go:build darwin

package machinestatus

import (
	"time"

	"golang.org/x/sys/unix"
)

// readPlatform は sysctl から、起動してからの時間と全体のメモリを読む。CPU の時間と使用中の
// メモリは Mach の呼び出しでしか読めず、cgo を使わない読み方が無いので持たない。
func readPlatform() Reading {
	var reading Reading
	if boot, err := unix.SysctlTimeval("kern.boottime"); err == nil {
		reading.Uptime = time.Since(time.Unix(boot.Unix()))
	}
	if total, err := unix.SysctlUint64("hw.memsize"); err == nil {
		reading.MemoryTotal = total
	}
	return reading
}
