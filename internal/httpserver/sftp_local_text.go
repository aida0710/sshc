package httpserver

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"
	sshcSFTP "sshc/internal/sftp"
)

func (h SFTPHandlers) ReadLocalText(c *echo.Context) error {
	file, err := sshcSFTP.ReadLocalText(c.Request().Context(), sshcSFTP.LocalTextReadOptions{Path: c.QueryParam("path"), ExpectedRevision: c.QueryParam("expectedRevision")})
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusOK, sftpTextFileResponse{Entry: describeSFTPEntry(file.Entry), Contents: file.Contents, Revision: file.Revision})
}

func (h SFTPHandlers) SaveLocalText(c *echo.Context) error {
	var body sftpSaveTextRequest
	if err := decodeJSON(c, &body); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	file, err := h.Transfers.SaveLocalText(c.Request().Context(), sshcSFTP.LocalTextSaveRequest{Path: c.QueryParam("path"), Contents: body.Contents, ExpectedRevision: body.ExpectedRevision})
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusOK, sftpTextFileResponse{Entry: describeSFTPEntry(file.Entry), Contents: file.Contents, Revision: file.Revision})
}

func (h SFTPHandlers) PreviewLocal(c *echo.Context) error {
	preview, err := sshcSFTP.ReadLocalPreview(c.Request().Context(), c.QueryParam("path"))
	if err != nil {
		return sftpProblem(c, err)
	}
	c.Response().Header().Set("ETag", strconv.Quote(preview.Revision))
	return c.Blob(http.StatusOK, preview.ContentType, preview.Contents)
}
