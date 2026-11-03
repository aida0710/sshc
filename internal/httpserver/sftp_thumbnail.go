package httpserver

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

// Thumbnail は、一覧の行に出す画像を返す。revision は一覧が示した版で、ファイルが変わったら別のサムネイルになる。
// 作れないファイルの理由は画面が読まない（アイコンのままにする）。
func (h SFTPHandlers) Thumbnail(c *echo.Context) error {
	thumbnail, err := h.Thumbnails.ForRemote(c.Request().Context(), c.Param("alias"), c.QueryParam("path"), c.QueryParam("revision"))
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.Blob(http.StatusOK, thumbnail.ContentType, thumbnail.Contents)
}

// ThumbnailLocal は、エンジンのディスクのファイルの Thumbnail である。
func (h SFTPHandlers) ThumbnailLocal(c *echo.Context) error {
	thumbnail, err := h.Thumbnails.ForLocal(c.Request().Context(), c.QueryParam("path"), c.QueryParam("revision"))
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.Blob(http.StatusOK, thumbnail.ContentType, thumbnail.Contents)
}
