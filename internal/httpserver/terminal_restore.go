package httpserver

import (
	"context"
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/platform"
	"sshc/internal/terminal"
	"sshc/internal/terminalrestore"
	"sshc/internal/terminalworkspace"
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
		if h.reopen(c.Request().Context(), saved) != nil {
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

// reopen opens one saved session. What changed since it was saved is let go
// rather than refused: a removed workspace becomes default, a directory that
// is gone becomes the start directory, and a shell profile this machine no
// longer has becomes the default shell.
func (h TerminalHandlers) reopen(ctx context.Context, saved terminalrestore.Session) error {
	request := api.OpenTerminalSessionRequest{Kind: api.OpenTerminalSessionRequestKind(saved.Kind)}
	if saved.Alias != "" {
		request.Alias = &saved.Alias
	}
	if saved.ShellProfile != "" {
		request.ProfileId = &saved.ShellProfile
	}
	if saved.WorkingDirectory != "" {
		request.Cwd = &saved.WorkingDirectory
	}
	spec, err := h.spec(request, terminal.Size{})
	if errors.Is(err, errInvalidRemoteWorkingDirectory) || errors.Is(err, terminal.ErrInvalidLocalWorkingDirectory) {
		request.Cwd = nil
		spec, err = h.spec(request, terminal.Size{})
	}
	if errors.Is(err, platform.ErrUnknownShellProfile) {
		request.ProfileId = nil
		spec, err = h.spec(request, terminal.Size{})
	}
	if err != nil {
		return err
	}
	spec.Workspace = saved.Workspace
	if h.requireWorkspace(saved.Workspace) != nil {
		spec.Workspace = terminalworkspace.DefaultID
	}
	session, err := h.Registry.Open(ctx, spec)
	if err != nil {
		return err
	}
	if saved.PinnedTitle != "" {
		_ = session.Rename(saved.PinnedTitle)
	}
	if saved.Kind == terminal.KindSSH && h.Connected != nil {
		session.WhenConnected(func() { h.Connected(saved.Alias) })
	}
	return nil
}
