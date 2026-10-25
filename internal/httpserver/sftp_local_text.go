package httpserver

import (
	"mime"
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
	if err := decodeJSONWithin(c, MaxSFTPTextJSONBodyCeiling, &body); err != nil {
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

// DownloadLocal はエンジンのディスクのファイルを、ブラウザが保存する形で返す。
// ServeContent が Range にも答えるが、型は名前から推さずに固定する。
func (h SFTPHandlers) DownloadLocal(c *echo.Context) error {
	download, err := sshcSFTP.OpenLocalDownload(c.QueryParam("path"))
	if err != nil {
		return sftpProblem(c, err)
	}
	defer download.File.Close()
	header := c.Response().Header()
	header.Set("Content-Type", "application/octet-stream")
	header.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": download.Name}))
	header.Set("ETag", strconv.Quote(download.Revision))
	http.ServeContent(c.Response(), c.Request(), "", download.ModTime, download.File)
	return nil
}
