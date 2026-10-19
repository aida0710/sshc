package httpserver

import (
	"net/http"

	"github.com/labstack/echo/v5"
	"sshc/internal/api"
	"sshc/internal/buildinfo"
)

type AboutHandlers struct {
	About api.About
}

func registerAboutRoute(engine *echo.Echo, version string, buildInfo *buildinfo.Info) {
	if buildInfo == nil {
		current := buildinfo.Read(version)
		buildInfo = &current
	}
	response := api.About{Version: buildInfo.Version, Revision: buildInfo.Revision, CommitTime: buildInfo.CommitTime,
		Modified: buildInfo.Modified, GoVersion: buildInfo.GoVersion, Os: buildInfo.OS, Arch: buildInfo.Arch,
		Manager: api.AboutManager(buildInfo.Manager), Executable: buildInfo.Executable}
	handlers := AboutHandlers{About: response}
	engine.GET("/api/v1/about", handlers.Read)
}

func (h *AboutHandlers) Read(c *echo.Context) error {
	return c.JSON(http.StatusOK, h.About)
}
