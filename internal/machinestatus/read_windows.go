//go:build windows

package machinestatus

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// x/sys/windows に無い2つを kernel32 から呼ぶ。
var (
	kernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	procGetSystemTimes       = kernel32.NewProc("GetSystemTimes")
	procGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
)

// memoryStatusEx は MEMORYSTATUSEX である。
type memoryStatusEx struct {
	length               uint32
	memoryLoad           uint32
	totalPhys            uint64
	availPhys            uint64
	totalPageFile        uint64
	availPageFile        uint64
	totalVirtual         uint64
	availVirtual         uint64
	availExtendedVirtual uint64
}

func readPlatform() Reading {
	reading := Reading{Uptime: windows.DurationSinceBoot()}
	var idle, kernel, user windows.Filetime
	if ok, _, _ := procGetSystemTimes.Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user))); ok != 0 {
		// kernel は idle を含む。
		total := filetimeTicks(kernel) + filetimeTicks(user)
		reading.CPU = &CPUTimes{Busy: total - filetimeTicks(idle), Total: total}
	}
	memory := memoryStatusEx{length: uint32(unsafe.Sizeof(memoryStatusEx{}))}
	if ok, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&memory))); ok != 0 && memory.availPhys <= memory.totalPhys {
		reading.MemoryTotal, reading.MemoryUsed = memory.totalPhys, memory.totalPhys-memory.availPhys
	}
	return reading
}

func filetimeTicks(value windows.Filetime) uint64 {
	return uint64(value.HighDateTime)<<32 | uint64(value.LowDateTime)
}
