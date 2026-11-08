// Package machinestatus は、エンジンが動いているマシンの状態（CPU・メモリ・稼働時間）を読む。
// ターミナルの空の状態に、新しいシェルが開くマシンの様子として出す。
//
// 読める値は OS によって違う。読めない値は持たず、画面はその値を出さない。
package machinestatus

import (
	"os"
	"runtime"
	"sync"
	"time"
)

// Reading は、ある時点のマシンの状態である。
type Reading struct {
	Hostname string
	// OS は runtime.GOOS（linux、darwin、windows、freebsd、openbsd、android）である。
	OS string
	// Uptime は、マシンが起動してからの時間である。読めなければ 0。
	Uptime time.Duration
	// CPU は、起動してからの CPU の時間の累計である。読めなければ nil。
	CPU *CPUTimes
	// MemoryTotal と MemoryUsed はバイトである。読めなければ 0。
	MemoryTotal uint64
	MemoryUsed  uint64
}

// CPUTimes は、CPU が働いた時間と全体の時間の累計である。単位は OS ごとに違い（Linux は
// クロックの刻み、Windows は 100 ナノ秒）、2 回の読み取りの差の比だけに使う。
type CPUTimes struct {
	Busy  uint64
	Total uint64
}

// Read は、今のマシンの状態を読む。
func Read() Reading {
	reading := readPlatform()
	reading.OS = runtime.GOOS
	if hostname, err := os.Hostname(); err == nil {
		reading.Hostname = hostname
	}
	return reading
}

// Status は、読み取りと、前の読み取りからの CPU の使用率（%）である。前の読み取りが無いか、
// CPU が読めない OS では CPUPercent は nil。
type Status struct {
	Reading
	CPUPercent *float64
}

// Sampler は、前に読んだ CPU の累計を覚え、次に読んだときとの差から使用率を出す。画面は数秒
// おきに読むので、使用率はその間の平均になる。エンジンに 1 つ置き、見ている画面が複数でも
// 共有する（そのときは、どれかの画面が前に読んでからの間の平均になる）。
type Sampler struct {
	read     func() Reading
	mutex    sync.Mutex
	previous *CPUTimes
}

func NewSampler(read func() Reading) *Sampler {
	if read == nil {
		read = Read
	}
	return &Sampler{read: read}
}

// Status は、今の状態を読む。
func (s *Sampler) Status() Status {
	reading := s.read()
	s.mutex.Lock()
	defer s.mutex.Unlock()
	status := Status{Reading: reading, CPUPercent: cpuPercent(s.previous, reading.CPU)}
	if reading.CPU != nil {
		s.previous = reading.CPU
	}
	return status
}

func cpuPercent(previous, current *CPUTimes) *float64 {
	if previous == nil || current == nil || current.Total <= previous.Total || current.Busy < previous.Busy {
		return nil
	}
	percent := float64(current.Busy-previous.Busy) / float64(current.Total-previous.Total) * 100
	percent = min(max(percent, 0), 100)
	return &percent
}
