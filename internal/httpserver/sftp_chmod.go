package httpserver

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/session"
	sshcSFTP "sshc/internal/sftp"
)

func chmodSelectionRequest(alias string, selection api.SFTPChmodSelection) (sshcSFTP.ChmodRequest, error) {
	fileMode, err := sshcSFTP.ParseChmodMode(selection.Options.FileMode)
	if err != nil {
		return sshcSFTP.ChmodRequest{}, err
	}
	directoryMode, err := sshcSFTP.ParseChmodMode(selection.Options.DirectoryMode)
	if err != nil {
		return sshcSFTP.ChmodRequest{}, err
	}
	entries := make([]sshcSFTP.ChmodEntry, len(selection.Entries))
	for index, entry := range selection.Entries {
		entries[index] = sshcSFTP.ChmodEntry{Path: entry.Path, ExpectedRevision: entry.ExpectedRevision}
	}
	return sshcSFTP.ChmodRequest{Alias: alias, Entries: entries, Options: sshcSFTP.ChmodOptions{
		FileMode: fileMode, DirectoryMode: directoryMode, Recursive: selection.Options.Recursive,
	}}, nil
}

func (h SFTPHandlers) PlanChmod(c *echo.Context) error {
	var body api.SFTPChmodSelection
	if err := decodeJSON(c, &body); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	request, err := chmodSelectionRequest(c.Param("alias"), body)
	if err != nil {
		return sftpProblem(c, err)
	}
	plan, err := h.Transfers.PrepareChmod(c.Request().Context(), request)
	if err != nil {
		return sftpProblem(c, err)
	}
	defer plan.Close()
	issued, allowed, response := h.Actions.issueEvidence(c, session.ActionSFTPChmod, request.Alias+":chmod:"+plan.Revision, plan.Revision)
	if !allowed {
		return response
	}
	expiresAt, err := time.Parse(time.RFC3339, issued.ExpiresAt)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, api.SFTPChmodPlan{Revision: plan.Revision,
		SelectionCount: len(body.Entries), Files: plan.Files, Directories: plan.Directories,
		SkippedSymlinks: plan.SkippedSymlinks, Options: body.Options,
		ActionToken: issued.Token, ActionExpiresAt: expiresAt,
	})
}

func (h SFTPHandlers) ChmodSelection(c *echo.Context) error {
	var body api.SFTPChmodSelectionRequest
	if err := decodeJSON(c, &body); err != nil || body.ExpectedRevision == "" {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if c.Request().Header.Get(ActionHeader) == "" {
		return problem(c, http.StatusForbidden, "action_token_required")
	}
	request, err := chmodSelectionRequest(c.Param("alias"), api.SFTPChmodSelection{Entries: body.Entries, Options: body.Options})
	if err != nil {
		return sftpProblem(c, err)
	}
	plan, err := h.Transfers.PrepareChmod(c.Request().Context(), request)
	if err != nil {
		return sftpProblem(c, err)
	}
	defer plan.Close()
	if plan.Revision != body.ExpectedRevision {
		return sftpProblem(c, sshcSFTP.ErrConflict)
	}
	if allowed, response := h.Actions.consumeEvidence(c, session.ActionSFTPChmod, request.Alias+":chmod:"+plan.Revision, plan.Revision); !allowed {
		return response
	}
	result, err := plan.Apply(c.Request().Context(), body.ExpectedRevision)
	if err != nil && !result.Started {
		return sftpProblem(c, err)
	}
	status := http.StatusOK
	if err != nil {
		status = http.StatusMultiStatus
	}
	return c.JSON(status, api.SFTPChmodResult{Applied: result.Applied, Items: result.Items, Complete: err == nil})
}

func (h SFTPHandlers) Chmod(c *echo.Context) error {
	var body sftpChmodRequest
	if err := decodeJSON(c, &body); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	mode, err := sshcSFTP.ParseChmodMode(body.Mode)
	if err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if c.Request().Header.Get(ActionHeader) == "" {
		return problem(c, http.StatusForbidden, "action_token_required")
	}
	alias := c.Param("alias")
	plan, err := h.Transfers.PrepareChmod(c.Request().Context(), sshcSFTP.ChmodRequest{
		Alias: alias, Entries: []sshcSFTP.ChmodEntry{{Path: body.Path, ExpectedRevision: body.ExpectedRevision}},
		Options: sshcSFTP.ChmodOptions{FileMode: mode, DirectoryMode: mode, Recursive: body.Recursive},
	})
	if err != nil {
		return sftpProblem(c, err)
	}
	defer plan.Close()
	if body.Recursive && plan.Directories == 0 {
		return sftpProblem(c, sshcSFTP.ErrNotDirectory)
	}
	target := alias + ":" + body.Path + ":" + body.Mode
	if body.Recursive {
		target += ":recursive"
	}
	if allowed, response := h.Actions.consumeEvidence(c, session.ActionSFTPChmod, target, plan.Revision); !allowed {
		return response
	}
	if _, err := plan.Apply(c.Request().Context(), plan.Revision); err != nil {
		return sftpProblem(c, err)
	}
	entry, err := plan.Entry(body.Path)
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusOK, describeSFTPEntry(entry))
}
