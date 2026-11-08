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
	"sshc/internal/recent"
	"sshc/internal/storage"
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

func TestConnectionHistoryGivesTheHoursAndTheRecentConnections(t *testing.T) {
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := func() time.Time { return time.Date(2026, 11, 8, 7, 20, 0, 0, time.UTC) }
	activity := recent.NewActivityStore(workspace, now)
	recentStore := recent.NewStore(workspace, now)
	for _, alias := range []string{"web-01", "db"} {
		if err := activity.Record(alias); err != nil {
			t.Fatal(err)
		}
		if err := recentStore.Record(alias); err != nil {
			t.Fatal(err)
		}
	}
	handlers := TerminalOverviewHandlers{
		Activity: activity,
		Recent: recent.NewService(recentStore, func(alias string) (recent.Target, error) {
			return recent.Target{Alias: alias, HostName: alias + ".example", User: "deploy", Port: "22"}, nil
		}),
	}

	var history api.ConnectionHistory
	callOverview(t, handlers.ConnectionHistory, "/api/v1/terminal/connection-history", &history)

	if len(history.Hours) != 1 || history.Hours[0].Connections != 2 || len(history.Hours[0].Aliases) != 2 ||
		!history.Hours[0].Hour.Equal(time.Date(2026, 11, 8, 7, 0, 0, 0, time.UTC)) {
		t.Fatalf("hours = %+v", history.Hours)
	}
	if len(history.Recent) != 2 || history.Recent[0].HostName != "web-01.example" && history.Recent[0].HostName != "db.example" {
		t.Fatalf("recent = %+v", history.Recent)
	}
}

// 画面の検証は hours と recent を配列として読むので、記録が無いときも null にしない。
func TestConnectionHistoryWithoutRecordsGivesEmptyLists(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/terminal/connection-history", nil)
	if err := (TerminalOverviewHandlers{}).ConnectionHistory(echo.New().NewContext(request, recorder)); err != nil {
		t.Fatal(err)
	}
	if body := recorder.Body.String(); body != "{\"hours\":[],\"recent\":[]}\n" {
		t.Fatalf("history = %s", body)
	}
}
