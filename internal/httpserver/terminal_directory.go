package httpserver

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/terminal"
)

// Directory answers which directory the program in the foreground of a
// session works in, so the screen can open a relative path it printed. The
// browser can already read any local file through the SFTP preview, so the
// answer exposes nothing new.
func (h TerminalHandlers) Directory(c *echo.Context) error {
	id, ok := sessionIDParam(c)
	if !ok {
		return problem(c, http.StatusNotFound, "terminal_session_not_found")
	}
	session, ok := h.Registry.Lookup(id)
	if !ok {
		return problem(c, http.StatusNotFound, "terminal_session_not_found")
	}
	directory, err := session.ForegroundDirectory()
	if errors.Is(err, terminal.ErrNotConnected) || errors.Is(err, terminal.ErrDirectoryUnavailable) {
		return problem(c, http.StatusConflict, "terminal_directory_unavailable")
	}
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, api.TerminalDirectory{Path: directory})
}
