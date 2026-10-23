package httpserver

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/selfupdate"
)

// Log は、いまの更新ジョブでインストーラーと再起動の helper が書いたものを cursor から返す。
// cursor と limit の決まりはターミナルの Control と同じである。
func (h *UpdateHandlers) Log(c *echo.Context) error {
	cursor, limit, ok := terminalControlRange(c)
	if !ok {
		return problem(c, http.StatusBadRequest, "invalid_terminal_cursor")
	}
	if h.Service == nil {
		return problem(c, http.StatusNotFound, "update_log_missing")
	}
	log, err := h.Service.Log(cursor, limit)
	switch {
	case errors.Is(err, selfupdate.ErrNoLog):
		return problem(c, http.StatusNotFound, "update_log_missing")
	case errors.Is(err, selfupdate.ErrLogCursorAhead):
		return problem(c, http.StatusConflict, "terminal_cursor_ahead")
	case err != nil:
		return problem(c, http.StatusInternalServerError, "update_state_failed")
	}
	return c.JSON(http.StatusOK, api.UpdateLog{
		JobId: log.JobID,
		Cursor: api.TerminalControlCursor{
			Requested: cursor, Start: log.Start, Next: log.Next, End: log.End,
			// 出力は先頭から上限まで残し、古い方を押し出さないので、読みたい位置が消えていることはない。
			Truncated: false,
		},
		ReachedCapacity: log.ReachedCapacity,
		Output:          log.Output,
	})
}
