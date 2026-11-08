package machinestatus

import (
	"encoding/binary"
	"testing"
	"time"
)

func TestTheFirstStatusHasNoCPUPercentAndTheNextIsTheShareOfBusyTimeBetweenThem(t *testing.T) {
	readings := []Reading{
		{CPU: &CPUTimes{Busy: 100, Total: 1000}},
		{CPU: &CPUTimes{Busy: 130, Total: 1200}},
	}
	sampler := NewSampler(func() Reading {
		reading := readings[0]
		readings = readings[1:]
		return reading
	})

	if first := sampler.Status(); first.CPUPercent != nil {
		t.Fatalf("first CPU percent = %v, want none until a second reading", *first.CPUPercent)
	}
	if second := sampler.Status(); second.CPUPercent == nil || *second.CPUPercent != 15 {
		t.Fatalf("second CPU percent = %v, want 15 (30 busy of 200)", second.CPUPercent)
	}
}

func TestAMachineWhoseCPUCannotBeReadNeverHasACPUPercent(t *testing.T) {
	sampler := NewSampler(func() Reading { return Reading{MemoryTotal: 1 << 30} })
	sampler.Status()

	if status := sampler.Status(); status.CPUPercent != nil {
		t.Fatalf("CPU percent = %v, want none", *status.CPUPercent)
	}
}

func TestCountersThatWentBackwardsGiveNoCPUPercent(t *testing.T) {
	if percent := cpuPercent(&CPUTimes{Busy: 500, Total: 2000}, &CPUTimes{Busy: 10, Total: 100}); percent != nil {
		t.Fatalf("CPU percent = %v, want none after the counters restarted", *percent)
	}
}

func TestProcStatCountsIdleAndIOWaitAsNotBusy(t *testing.T) {
	times, err := parseProcStat([]byte("cpu  100 5 50 800 20 3 2 0 40 0\ncpu0 50 2 25 400 10 1 1 0 20 0\n"))
	if err != nil {
		t.Fatal(err)
	}
	// guest（40）は user に含まれているので足さない。
	if times.Total != 980 || times.Busy != 160 {
		t.Fatalf("times = %+v, want total 980 and busy 160", times)
	}
}

func TestMeminfoUsedIsTotalLessAvailable(t *testing.T) {
	total, used, err := parseMeminfo([]byte("MemTotal:       32000000 kB\nMemFree:         1000000 kB\nMemAvailable:   24000000 kB\nBuffers:          100 kB\n"))
	if err != nil {
		t.Fatal(err)
	}
	if total != 32000000*1024 || used != 8000000*1024 {
		t.Fatalf("total = %d, used = %d", total, used)
	}
}

func TestMeminfoWithoutMemAvailableIsNotGuessed(t *testing.T) {
	if _, _, err := parseMeminfo([]byte("MemTotal: 32000000 kB\nMemFree: 1000000 kB\n")); err == nil {
		t.Fatal("parseMeminfo accepted a file without MemAvailable")
	}
}

func TestUptimeIsTheFirstNumberInSeconds(t *testing.T) {
	uptime, err := parseUptime([]byte("1036800.52 8000000.10\n"))
	if err != nil {
		t.Fatal(err)
	}
	if uptime.Round(time.Second) != 12*24*time.Hour+time.Second {
		t.Fatalf("uptime = %v, want 12 days and a second", uptime)
	}
}

func TestCPTimeTakesTheLastStateAsIdle(t *testing.T) {
	for _, longSize := range []int{4, 8} {
		values := []uint64{100, 5, 50, 3, 842}
		raw := make([]byte, 0, len(values)*longSize)
		for _, value := range values {
			if longSize == 8 {
				raw = binary.NativeEndian.AppendUint64(raw, value)
			} else {
				raw = binary.NativeEndian.AppendUint32(raw, uint32(value))
			}
		}
		times, ok := parseCPTime(raw, longSize)
		if !ok || times.Total != 1000 || times.Busy != 158 {
			t.Fatalf("long size %d: times = %+v, ok = %v", longSize, times, ok)
		}
	}
}
