package httpserver

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
	"sshc/internal/api"
	"sshc/internal/session"
	sshcSFTP "sshc/internal/sftp"
)

func (h SFTPHandlers) MkdirLocal(c *echo.Context) error {
	var body api.SFTPLocalMkdirRequest
	if err := decodeJSON(c, &body); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	entry, err := h.Transfers.MkdirLocal(c.Request().Context(), sshcSFTP.LocalMkdirRequest{Directory: body.Directory, Name: body.Name})
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusCreated, describeSFTPEntry(entry))
}

func (h SFTPHandlers) RenameLocal(c *echo.Context) error {
	var body api.SFTPLocalRenameRequest
	if err := decodeJSON(c, &body); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	entry, err := h.Transfers.RenameLocal(c.Request().Context(), sshcSFTP.LocalRenameRequest{Path: body.Path, Name: body.Name, ExpectedRevision: body.ExpectedRevision})
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusOK, describeSFTPEntry(entry))
}

func localDeleteSelection(entries []api.SFTPLocalDeleteEntry) []sshcSFTP.LocalDeleteEntry {
	selection := make([]sshcSFTP.LocalDeleteEntry, len(entries))
	for index, entry := range entries {
		selection[index] = sshcSFTP.LocalDeleteEntry{Path: entry.Path, ExpectedRevision: entry.ExpectedRevision}
	}
	return selection
}

func (h SFTPHandlers) PlanLocalDelete(c *echo.Context) error {
	var body api.SFTPLocalDeleteSelection
	if err := decodeJSON(c, &body); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	plan, err := h.Transfers.PrepareLocalDelete(c.Request().Context(), localDeleteSelection(body.Entries))
	if err != nil {
		return sftpProblem(c, err)
	}
	defer plan.Close()
	issued, allowed, response := h.Actions.issueEvidence(c, session.ActionSFTPDelete, "local:"+plan.Revision, plan.Revision)
	if !allowed {
		return response
	}
	expiresAt, err := time.Parse(time.RFC3339, issued.ExpiresAt)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, api.SFTPLocalDeletePlan{Revision: plan.Revision, Items: plan.Items, ActionToken: issued.Token, ActionExpiresAt: expiresAt})
}

func (h SFTPHandlers) DeleteLocal(c *echo.Context) error {
	var body api.SFTPLocalDeleteRequest
	if err := decodeJSON(c, &body); err != nil || body.ExpectedRevision == "" {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if c.Request().Header.Get(ActionHeader) == "" {
		return problem(c, http.StatusForbidden, "action_token_required")
	}
	plan, err := h.Transfers.PrepareLocalDelete(c.Request().Context(), localDeleteSelection(body.Entries))
	if err != nil {
		return sftpProblem(c, err)
	}
	defer plan.Close()
	if body.ExpectedRevision != plan.Revision {
		return sftpProblem(c, sshcSFTP.ErrConflict)
	}
	if allowed, response := h.Actions.consumeEvidence(c, session.ActionSFTPDelete, "local:"+plan.Revision, plan.Revision); !allowed {
		return response
	}
	if err := plan.Delete(c.Request().Context(), body.ExpectedRevision); err != nil {
		return sftpProblem(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}
