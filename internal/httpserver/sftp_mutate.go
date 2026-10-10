package httpserver

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
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
