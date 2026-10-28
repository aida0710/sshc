package httpserver

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
)

func registerTerminalRestoreRoutes(engine *echo.Echo, handlers TerminalHandlers) {
	engine.GET("/api/v1/terminal/restore", handlers.RestoreOffer)
	engine.POST("/api/v1/terminal/restore", handlers.Restore)
	engine.DELETE("/api/v1/terminal/restore", handlers.DeclineRestore)
}

// RestoreOffer returns the sessions the previous engine left open. The screen
// asks the user whether to open them again.
func (h TerminalHandlers) RestoreOffer(c *echo.Context) error {
	offer := h.RestoreOffers.Offer()
	sessions := make([]api.TerminalRestoreSession, 0, len(offer))
	for _, session := range offer {
		described := api.TerminalRestoreSession{
			Kind: api.TerminalRestoreSessionKind(session.Kind), WorkspaceId: session.Workspace,
		}
		if session.Alias != "" {
			described.Alias = &session.Alias
		}
		if session.PinnedTitle != "" {
			described.PinnedTitle = &session.PinnedTitle
		}
		if session.WorkingDirectory != "" {
			described.WorkingDirectory = &session.WorkingDirectory
		}
		sessions = append(sessions, described)
	}
	return c.JSON(http.StatusOK, api.TerminalRestoreOffer{Sessions: sessions})
}

// Restore ends the offer and opens its sessions in their saved order. One that
// cannot be opened is counted and skipped, so the rest still open.
func (h TerminalHandlers) Restore(c *echo.Context) error {
	offer, err := h.RestoreOffers.Take()
	if err != nil {
		return unexpectedProblem(c, "terminal_restore_failed", err)
	}
	result := api.TerminalRestoreResult{}
	for _, saved := range offer {
		if _, err := h.opener().Reopen(c.Request().Context(), saved); err != nil {
			result.Failed++
			continue
		}
		result.Opened++
	}
	return c.JSON(http.StatusOK, result)
}

// DeclineRestore ends the offer without opening anything.
func (h TerminalHandlers) DeclineRestore(c *echo.Context) error {
	if err := h.RestoreOffers.Decline(); err != nil {
		return unexpectedProblem(c, "terminal_restore_failed", err)
	}
	return c.NoContent(http.StatusNoContent)
}
