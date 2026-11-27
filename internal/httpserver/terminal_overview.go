package httpserver

import (
	"math"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/machinestatus"
)

// TerminalOverviewHandlers は、Home の一番下に出す、このエンジンが動くマシンの状態を返す。実行先の
// エンジンのものは registerRemoteEngineRoutes が取り次ぐ。
type TerminalOverviewHandlers struct {
	Machine *machinestatus.Sampler
}

func registerTerminalOverviewRoutes(engine *echo.Echo, handlers TerminalOverviewHandlers) {
	engine.GET("/api/v1/terminal/machine", handlers.MachineStatus)
}

func (h TerminalOverviewHandlers) MachineStatus(c *echo.Context) error {
	var status machinestatus.Status
	if h.Machine != nil {
		status = h.Machine.Status()
	} else {
		status = machinestatus.Status{Reading: machinestatus.Read()}
	}
	answer := api.MachineStatus{Hostname: status.Hostname, Os: status.OS}
	if status.Uptime > 0 {
		answer.UptimeSeconds = optionalInt64(uint64(status.Uptime / time.Second))
	}
	if status.CPUPercent != nil {
		percent := float32(*status.CPUPercent)
		answer.CpuPercent = &percent
	}
	if status.MemoryTotal > 0 {
		answer.MemoryTotalBytes = optionalInt64(status.MemoryTotal)
	}
	if status.MemoryUsed > 0 {
		answer.MemoryUsedBytes = optionalInt64(status.MemoryUsed)
	}
	return c.JSON(http.StatusOK, answer)
}

func optionalInt64(value uint64) *int64 {
	clamped := int64(min(value, math.MaxInt64))
	return &clamped
}
