package httpserver

import (
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
	var media sshcSFTP.MediaFile
	var err error
	if claim.alias == sshcSFTP.LocalAlias {
		media, err = sshcSFTP.OpenLocalMedia(claim.path)
	} else {
		media, err = h.Service.OpenMedia(c.Request().Context(), claim.alias, claim.path)
	}
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
