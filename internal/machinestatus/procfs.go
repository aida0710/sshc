package machinestatus

import (
	"bufio"
	"bytes"
	"errors"
	"strconv"
	"strings"
	"time"
)

// Linux と Android の /proc の読み方。ファイルを読むのは read_linux.go で、ここは中身を解釈する
// だけなので、どの OS のテストからも確かめられる。

var errUnreadableProc = errors.New("the /proc file is not in the expected form")

// parseProcStat は /proc/stat の先頭の cpu 行を読む。値の並びは user nice system idle iowait irq
// softirq steal（guest と guest_nice は user と nice に含まれるので足さない）。働いていないのは
// idle と iowait である。
func parseProcStat(contents []byte) (CPUTimes, error) {
	line, _, _ := bytes.Cut(contents, []byte("\n"))
	fields := strings.Fields(string(line))
	if len(fields) < 5 || fields[0] != "cpu" {
		return CPUTimes{}, errUnreadableProc
	}
	var values []uint64
	for _, field := range fields[1:min(len(fields), 9)] {
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return CPUTimes{}, errUnreadableProc
		}
		values = append(values, value)
	}
	var total uint64
	for _, value := range values {
		total += value
	}
	idle := values[3]
	if len(values) > 4 {
		idle += values[4]
	}
	return CPUTimes{Busy: total - idle, Total: total}, nil
}

// parseMeminfo は /proc/meminfo の全体と使用中のメモリを読む。使用中は MemTotal から
// MemAvailable（すぐ使える量）を引いたもので、free コマンドの used に近い。
func parseMeminfo(contents []byte) (total, used uint64, err error) {
	values := map[string]uint64{}
	scanner := bufio.NewScanner(bytes.NewReader(contents))
	for scanner.Scan() {
		name, rest, ok := strings.Cut(scanner.Text(), ":")
		if !ok {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		value, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}
		// 値の単位は kB（1024 バイト）である。
		values[name] = value * 1024
	}
	total, hasTotal := values["MemTotal"]
	available, hasAvailable := values["MemAvailable"]
	if !hasTotal || !hasAvailable || available > total {
		return 0, 0, errUnreadableProc
	}
	return total, total - available, nil
}

// parseUptime は /proc/uptime の先頭の値（起動してからの秒）を読む。
func parseUptime(contents []byte) (time.Duration, error) {
	fields := strings.Fields(string(contents))
	if len(fields) == 0 {
		return 0, errUnreadableProc
	}
	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || seconds < 0 {
		return 0, errUnreadableProc
	}
	return time.Duration(seconds * float64(time.Second)), nil
}
