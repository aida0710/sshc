package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/machinestatus"
)

func callOverview(t *testing.T, handler func(*echo.Context) error, path string, answer any) {
	t.Helper()
	recorder := httptest.NewRecorder()
	if err := handler(echo.New().NewContext(httptest.NewRequest(http.MethodGet, path, nil), recorder)); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("%s = %d, body %s", path, recorder.Code, recorder.Body.String())
	}
	decoder := json.NewDecoder(bytes.NewReader(recorder.Body.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(answer); err != nil {
		t.Fatal(err)
	}
}

func TestMachineStatusLeavesOutWhatTheMachineDoesNotGive(t *testing.T) {
	readings := []machinestatus.Reading{
		{Hostname: "workstation", OS: "darwin", Uptime: 36 * time.Hour, MemoryTotal: 16 << 30},
	}
	handlers := TerminalOverviewHandlers{Machine: machinestatus.NewSampler(func() machinestatus.Reading { return readings[0] })}

	var status api.MachineStatus
	callOverview(t, handlers.MachineStatus, "/api/v1/terminal/machine", &status)

	if status.Hostname != "workstation" || status.Os != "darwin" || status.UptimeSeconds == nil || *status.UptimeSeconds != 36*60*60 {
		t.Fatalf("status = %+v", status)
	}
	if status.MemoryTotalBytes == nil || *status.MemoryTotalBytes != 16<<30 {
		t.Fatalf("memory total = %v, want 16 GiB", status.MemoryTotalBytes)
	}
	if status.CpuPercent != nil || status.MemoryUsedBytes != nil {
		t.Fatalf("status = %+v, want no CPU use and no used memory from a machine that gives neither", status)
	}
}

func TestMachineStatusGivesTheCPUUseSinceThePreviousRead(t *testing.T) {
	readings := []machinestatus.Reading{
		{Hostname: "r540", OS: "linux", CPU: &machinestatus.CPUTimes{Busy: 0, Total: 0}},
		{Hostname: "r540", OS: "linux", CPU: &machinestatus.CPUTimes{Busy: 25, Total: 100}, MemoryTotal: 8 << 30, MemoryUsed: 2 << 30},
	}
	handlers := TerminalOverviewHandlers{Machine: machinestatus.NewSampler(func() machinestatus.Reading {
		reading := readings[0]
		readings = readings[1:]
		return reading
	})}

	var first, second api.MachineStatus
	callOverview(t, handlers.MachineStatus, "/api/v1/terminal/machine", &first)
	callOverview(t, handlers.MachineStatus, "/api/v1/terminal/machine", &second)

	if first.CpuPercent != nil {
		t.Fatalf("first CPU use = %v, want none", *first.CpuPercent)
	}
	if second.CpuPercent == nil || *second.CpuPercent != 25 || second.MemoryUsedBytes == nil || *second.MemoryUsedBytes != 2<<30 {
		t.Fatalf("second = %+v, want 25%% CPU and 2 GiB used", second)
	}
}
