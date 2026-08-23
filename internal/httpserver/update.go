package httpserver

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/selfupdate"
)

// UpdateHandlers は現在のバージョンと利用可能な最新リリースを報告する。
// 更新の確認だけを行い、バイナリのダウンロードや置換は行わない。
type UpdateHandlers struct {
	Current string
	// Checker は、このビルドが自身と比較すべきものを持たない場合に
	// nil になる。その場合バージョンだけが報告され、他は何も報告されない。
	Checker *selfupdate.Checker
}

func registerUpdateRoutes(engine *echo.Echo, handlers *UpdateHandlers) {
	engine.GET("/api/v1/update", handlers.Check)
}

func (h *UpdateHandlers) answer(c *echo.Context, latest selfupdate.Release, available bool) error {
	status := api.UpdateStatus{Current: h.Current, Available: available}
	if latest.Version != "" {
		version, page := latest.Version, latest.PageURL
		status.Latest, status.PageUrl = &version, &page
	}
	return c.JSON(http.StatusOK, status)
}

// Check は最新のリリースが何かを尋ねる。
func (h *UpdateHandlers) Check(c *echo.Context) error {
	if h.Checker == nil {
		return h.answer(c, selfupdate.Release{}, false)
	}
	latest, err := h.Checker.Latest(c.Request().Context())
	switch {
	case errors.Is(err, selfupdate.ErrNoRelease):
		return h.answer(c, selfupdate.Release{}, false)
	case err != nil:
		return problem(c, http.StatusBadGateway, "update_check_failed")
	}
	return h.answer(c, latest, selfupdate.Newer(h.Current, latest.Version))
}
