package httpserver

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"
)

func (h SFTPHandlers) ReadText(c *echo.Context) error {
	file, err := h.Service.ReadText(c.Request().Context(), c.Param("alias"), c.QueryParam("path"))
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusOK, sftpTextFileResponse{
		Entry: describeSFTPEntry(file.Entry), Contents: file.Contents, Revision: file.Revision,
	})
}

// Preview は、詳細モーダルが描く画像そのものを返す。
//
// 名乗る型は Service が中身から決めたものだけであり、SFTP server の申告でも
// 拡張子でもない。X-Content-Type-Options は Security.Middleware が全応答へ
// 付けているので、ここで名乗った型より先へブラウザが推測することはない。
func (h SFTPHandlers) Preview(c *echo.Context) error {
	preview, err := h.Service.ReadPreview(c.Request().Context(), c.Param("alias"), c.QueryParam("path"))
	if err != nil {
		return sftpProblem(c, err)
	}
	c.Response().Header().Set("ETag", strconv.Quote(preview.Revision))
	return c.Blob(http.StatusOK, preview.ContentType, preview.Contents)
}

func (h SFTPHandlers) SaveText(c *echo.Context) error {
	var body sftpSaveTextRequest
	if err := decodeJSON(c, &body); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	file, err := h.Transfers.SaveText(c.Request().Context(), c.Param("alias"), c.QueryParam("path"), body.Contents, body.ExpectedRevision)
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusOK, sftpTextFileResponse{
		Entry: describeSFTPEntry(file.Entry), Contents: file.Contents, Revision: file.Revision,
	})
}
