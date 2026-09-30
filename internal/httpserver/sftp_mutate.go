package httpserver

import (
	"io/fs"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/session"
	sshcSFTP "sshc/internal/sftp"
)

func (h SFTPHandlers) CreateEntry(c *echo.Context) error {
	var body api.SFTPCreateEntryRequest
	if err := decodeJSON(c, &body); err != nil || !body.Type.Valid() {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	alias := c.Param("alias")
	unlock, err := h.Transfers.LockOperation(alias, body.Path)
	if err != nil {
		return sftpProblem(c, err)
	}
	defer unlock()
	var entry sshcSFTP.Entry
	if body.Type == api.SFTPCreateEntryRequestTypeDirectory {
		entry, err = h.Service.Mkdir(c.Request().Context(), alias, body.Path)
	} else {
		entry, err = h.Service.CreateEmptyFile(c.Request().Context(), alias, body.Path)
	}
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusCreated, describeSFTPEntry(entry))
}

func (h SFTPHandlers) Rename(c *echo.Context) error {
	var body sftpRenameRequest
	if err := decodeJSON(c, &body); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	alias := c.Param("alias")
	unlock, err := h.Transfers.LockOperation(alias, body.From, body.To)
	if err != nil {
		return sftpProblem(c, err)
	}
	defer unlock()
	entry, err := h.Service.Rename(c.Request().Context(), alias, body.From, body.To)
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusOK, describeSFTPEntry(entry))
}

func (h SFTPHandlers) Chmod(c *echo.Context) error {
	var body sftpChmodRequest
	if err := decodeJSON(c, &body); err != nil || (len(body.Mode) != 3 && len(body.Mode) != 4) {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	parsed, err := strconv.ParseUint(body.Mode, 8, 12)
	if err != nil || parsed > 0o777 {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	alias := c.Param("alias")
	unlock, err := h.Transfers.LockOperation(alias, body.Path)
	if err != nil {
		return sftpProblem(c, err)
	}
	defer unlock()
	target := alias + ":" + body.Path + ":" + body.Mode
	if body.Recursive {
		target += ":recursive"
	}
	if allowed, response := h.Actions.consume(c, session.ActionSFTPChmod, target); !allowed {
		return response
	}
	var entry sshcSFTP.Entry
	if body.Recursive {
		entry, err = h.Service.ChmodRecursive(c.Request().Context(), alias, body.Path, fs.FileMode(parsed), body.ExpectedRevision)
	} else {
		entry, err = h.Service.Chmod(c.Request().Context(), alias, body.Path, fs.FileMode(parsed), body.ExpectedRevision)
	}
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusOK, describeSFTPEntry(entry))
}
