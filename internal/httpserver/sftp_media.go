package httpserver

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v5"

	sshcSFTP "sshc/internal/sftp"
)

// sftpMediaRoute is the one API read that does not carry the CSRF header; its
// ticket does that job (mediaTickets).
const sftpMediaRoute = "/api/v1/sftp/media/:ticket"

type sftpMediaTicketRequest struct {
	Alias string `json:"alias"`
	Path  string `json:"path"`
}

type sftpMediaTicketResponse struct {
	Ticket string `json:"ticket"`
}

func sessionOf(c *echo.Context) string {
	session, _ := c.Get(SessionContextKey).(string)
	return session
}

// IssueMediaTicket lets the preview's player read one audio or video file.
// Only names that play are accepted, so a ticket never serves anything else.
func (h SFTPHandlers) IssueMediaTicket(c *echo.Context) error {
	var body sftpMediaTicketRequest
	if err := decodeJSONWithin(c, MaxRequestBodyCeiling, &body); err != nil || body.Alias == "" || body.Path == "" {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if sshcSFTP.MediaContentType(body.Path) == "" {
		return problem(c, http.StatusUnsupportedMediaType, "sftp_preview_type")
	}
	// The file is opened once before the ticket is given, so a missing or unreadable file is answered now with
	// its reason. Once the player has the ticket, every failure reaches the page as the same player error.
	media, err := h.openMedia(c.Request().Context(), body.Alias, body.Path)
	if err != nil {
		return sftpProblem(c, err)
	}
	if err := media.Close(); err != nil {
		return sftpProblem(c, err)
	}
	ticket, err := h.MediaTickets.issue(sessionOf(c), body.Alias, body.Path)
	if err != nil {
		return unexpectedProblem(c, "sftp_failed", err)
	}
	return c.JSON(http.StatusCreated, sftpMediaTicketResponse{Ticket: ticket})
}

func (h SFTPHandlers) RevokeMediaTicket(c *echo.Context) error {
	h.MediaTickets.revoke(sessionOf(c), c.Param("ticket"))
	return c.NoContent(http.StatusNoContent)
}

// StreamMedia sends the ranges a player asks for. ServeContent answers Range,
// including the closed ranges Safari probes with before it plays.
func (h SFTPHandlers) StreamMedia(c *echo.Context) error {
	claim, ok := h.MediaTickets.claim(sessionOf(c), c.Param("ticket"))
	if !ok {
		return problem(c, http.StatusNotFound, "sftp_not_found")
	}
	media, err := h.openMedia(c.Request().Context(), claim.alias, claim.path)
	if err != nil {
		return sftpProblem(c, err)
	}
	defer media.Close()
	header := c.Response().Header()
	header.Set("Content-Type", media.ContentType)
	header.Set("Content-Disposition", "inline")
	http.ServeContent(c.Response(), c.Request(), "", media.ModTime, media.Content)
	return nil
}

// openMedia opens an audio or video file on this machine or over SFTP.
func (h SFTPHandlers) openMedia(ctx context.Context, alias, path string) (sshcSFTP.MediaFile, error) {
	if alias == sshcSFTP.LocalAlias {
		return sshcSFTP.OpenLocalMedia(path)
	}
	return h.Service.OpenMedia(ctx, alias, path)
}
