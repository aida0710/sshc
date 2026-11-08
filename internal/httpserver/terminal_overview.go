package httpserver

import (
	"math"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/machinestatus"
	"sshc/internal/recent"
)

// TerminalOverviewHandlers は、ターミナルの空の状態に出す、このエンジンが動くマシンの状態と
// ターミナルの SSH 接続の記録を返す。実行先のエンジンのものは registerRemoteEngineRoutes が
// 取り次ぐ。
type TerminalOverviewHandlers struct {
	Machine  *machinestatus.Sampler
	Activity *recent.ActivityStore
	Recent   *recent.Service
}

func registerTerminalOverviewRoutes(engine *echo.Echo, handlers TerminalOverviewHandlers) {
	engine.GET("/api/v1/terminal/machine", handlers.MachineStatus)
	engine.GET("/api/v1/terminal/connection-history", handlers.ConnectionHistory)
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

func (h TerminalOverviewHandlers) ConnectionHistory(c *echo.Context) error {
	history := api.ConnectionHistory{Hours: []api.ConnectionHour{}, Recent: []api.RecentConnection{}}
	if h.Activity != nil {
		hours, err := h.Activity.List()
		if err != nil {
			return unexpectedProblem(c, "connection_history_failed", err)
		}
		for _, hour := range hours {
			aliases := hour.Aliases
			if aliases == nil {
				aliases = []string{}
			}
			history.Hours = append(history.Hours, api.ConnectionHour{Hour: hour.Hour, Connections: hour.Connections, Aliases: aliases})
		}
	}
	if h.Recent != nil {
		connections, err := h.Recent.List()
		if err != nil {
			return unexpectedProblem(c, "connection_history_failed", err)
		}
		history.Recent = recentConnectionsForAPI(connections)
	}
	return c.JSON(http.StatusOK, history)
}
