//go:build linux

package machinestatus

import "os"

// readPlatform は /proc から読む。Android のアプリには /proc/stat を読ませない端末があり、
// そのときは CPU を持たない。
func readPlatform() Reading {
	var reading Reading
	if contents, err := os.ReadFile("/proc/stat"); err == nil {
		if times, err := parseProcStat(contents); err == nil {
			reading.CPU = &times
		}
	}
	if contents, err := os.ReadFile("/proc/meminfo"); err == nil {
		if total, used, err := parseMeminfo(contents); err == nil {
			reading.MemoryTotal, reading.MemoryUsed = total, used
		}
	}
	if contents, err := os.ReadFile("/proc/uptime"); err == nil {
		if uptime, err := parseUptime(contents); err == nil {
			reading.Uptime = uptime
		}
	}
	return reading
}
