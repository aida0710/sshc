package httpserver

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/terminal"
)

const (
	defaultTerminalControlReadBytes = 32 << 10
	maxTerminalControlReadBytes     = 64 << 10
)

// Control returns explicit lifecycle state and a cursor-addressed, bounded
// plain-text scrollback fragment for CLI automation.
func (h TerminalHandlers) Control(c *echo.Context) error {
	id := c.Param("id")
	if id == "" || len(id) > maxSessionIdentifier {
		return problem(c, http.StatusNotFound, "terminal_session_not_found")
	}
	session, ok := h.Registry.Lookup(id)
	if !ok {
		return problem(c, http.StatusNotFound, "terminal_session_not_found")
	}
	cursor, limit, ok := terminalControlRange(c)
	if !ok {
		return problem(c, http.StatusBadRequest, "invalid_terminal_cursor")
	}
	snapshot, ok := session.ReadControl(cursor, limit)
	if !ok {
		return problem(c, http.StatusConflict, "terminal_cursor_ahead")
	}
	return c.JSON(http.StatusOK, api.TerminalControlResponse{
		Session: describeSession(session.View()), Generation: snapshot.Generation,
		State: api.TerminalControlResponseState(snapshot.State),
		Cursor: api.TerminalControlCursor{
			Requested: cursor, Start: snapshot.Read.Start, Next: snapshot.Read.Next,
			End: snapshot.Read.End, Truncated: snapshot.Read.Truncated,
		},
		Output: terminal.PlainTextFrom(snapshot.Read.Context, snapshot.Read.Emit),
	})
}

func terminalControlRange(c *echo.Context) (uint64, int, bool) {
	cursor := uint64(0)
	if raw := c.QueryParam("cursor"); raw != "" {
		parsed, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return 0, 0, false
		}
		cursor = parsed
	}
	limit := defaultTerminalControlReadBytes
	if raw := c.QueryParam("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 || parsed > maxTerminalControlReadBytes {
			return 0, 0, false
		}
		limit = parsed
	}
	return cursor, limit, true
}
