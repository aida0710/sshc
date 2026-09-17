package httpserver

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/remotesync"
	"sshc/internal/secret"
)

// The automatic sync loop: its reported state, the switch and "run now".

// autoResponse は、巡回の現在地を画面の形にする。
func (h SyncHandlers) autoResponse() api.AutoSync {
	if h.Auto == nil {
		return api.AutoSync{Enabled: false, Phase: api.AutoSyncPhase(remotesync.AutoIdle)}
	}
	view := h.Auto.View()
	response := api.AutoSync{Enabled: view.Enabled, Phase: api.AutoSyncPhase(view.Phase)}
	if view.Detail != "" {
		detail := view.Detail
		response.Detail = &detail
	}
	if view.At != "" {
		at := view.At
		response.At = &at
	}
	return response
}

// SetAuto は、巡回の入切を決める。
//
// 切ったことも保管庫の中に残る。この実行のあいだだけ止まる切り方は、次に
// 起動したときに暗黙に再開することであり、止めたユーザーはそれを止めたと思っている。
func (h SyncHandlers) SetAuto(c *echo.Context) error {
	var request api.AutoSyncRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if h.Secrets == nil {
		return problem(c, http.StatusConflict, "vault_locked")
	}
	if err := h.Secrets.SetSyncAuto(request.Enabled); err != nil {
		if errors.Is(err, secret.ErrLocked) || errors.Is(err, secret.ErrNoVault) {
			return problem(c, http.StatusConflict, "vault_locked")
		}
		return problem(c, http.StatusInternalServerError, "vault_failed")
	}
	if request.Enabled && h.Auto != nil {
		h.restore()
		h.Auto.Poll(c.Request().Context())
	}
	return h.status(c)
}

// Now は、一巡を押したユーザーを待たせたまま行う。
//
// 自動同期の入切とは独立した、明示的な一巡である。
func (h SyncHandlers) Now(c *echo.Context) error {
	h.restore()
	if h.Auto == nil {
		return problem(c, http.StatusConflict, "auto_sync_off")
	}
	view := h.Auto.Now(c.Request().Context())
	if view.Phase == remotesync.AutoFailed {
		return autoSyncFailureProblem(c, view.Detail)
	}
	return h.status(c)
}
