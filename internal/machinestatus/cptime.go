package machinestatus

import "encoding/binary"

// parseCPTime は、BSD の kern.cp_time（状態ごとの刻みの long の並び、longSize バイトずつ）を
// 読む。並びの最後が idle である（FreeBSD は user nice sys intr idle、OpenBSD は user nice sys
// spin intr idle）。読むのは read_bsd.go だが、どの OS のテストからも確かめられるようにここに置く。
func parseCPTime(raw []byte, longSize int) (CPUTimes, bool) {
	if (longSize != 4 && longSize != 8) || len(raw) < 2*longSize || len(raw)%longSize != 0 {
		return CPUTimes{}, false
	}
	var total, idle uint64
	for offset := 0; offset < len(raw); offset += longSize {
		var value uint64
		if longSize == 8 {
			value = binary.NativeEndian.Uint64(raw[offset:])
		} else {
			value = uint64(binary.NativeEndian.Uint32(raw[offset:]))
		}
		total += value
		idle = value
	}
	return CPUTimes{Busy: total - idle, Total: total}, true
}
