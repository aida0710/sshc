package httpserver

import (
	"errors"
	"io"
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/application"
)

// 端末の背景画像。
//
// **名前を決めるのはサーバーである。** 送られてくるのは希望と中身だけで、
// 実際の綴りと型は応答が答える。送られた綴りをそのままファイル名にすれば、
// `../` も隠しファイルも拡張子の詐称もそこから入る。
//
// 画像は封の中を旅する（remotesync.Collect）。**Android はサンドボックスの外を
// 見られない**ので、あの端末へ画像を持ち込む道はこれしかない。

func registerBackgroundRoutes(engine *echo.Echo, handlers ConfigHandlers) {
	engine.GET("/api/v1/terminal/backgrounds", handlers.Backgrounds)
	engine.POST("/api/v1/terminal/backgrounds", handlers.AddBackground)
	engine.GET("/api/v1/terminal/backgrounds/:name", handlers.Background)
	engine.DELETE("/api/v1/terminal/backgrounds/:name", handlers.DeleteBackground)
}

// backgroundList は、置いてある画像と、あと何バイト置けるかを返す。
//
// **残りを数えるのはこちらである。** 画面が上限を書き写すと、上限を変えた日に
// 画面だけが古い数を信じる。
func (h ConfigHandlers) Backgrounds(c *echo.Context) error {
	backgrounds, err := h.Service.Backgrounds()
	if err != nil {
		return problem(c, http.StatusInternalServerError, "backgrounds_unreadable")
	}
	used := 0
	for _, background := range backgrounds {
		used += background.Bytes
	}
	remaining := application.MaxBackgroundsBytes - used
	if remaining < 0 {
		remaining = 0
	}
	return c.JSON(http.StatusOK, map[string]any{
		"backgrounds":    backgrounds,
		"remainingBytes": remaining,
	})
}

func (h ConfigHandlers) AddBackground(c *echo.Context) error {
	body := c.Request().Body
	if body == nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	// **1 バイト余分に読む。** ちょうど上限で切ると、超えていることと
	// ちょうど収まっていることが見分けられない。
	contents, err := io.ReadAll(io.LimitReader(body, application.MaxBackgroundBytes+1))
	if err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	background, err := h.Service.AddBackground(c.QueryParam("name"), contents)
	switch {
	case errors.Is(err, application.ErrBackgroundTooLarge):
		return problem(c, http.StatusRequestEntityTooLarge, "background_too_large")
	case errors.Is(err, application.ErrBackgroundsFull):
		return problem(c, http.StatusRequestEntityTooLarge, "backgrounds_full")
	case errors.Is(err, application.ErrNotAnImage):
		return problem(c, http.StatusBadRequest, "not_an_image")
	case err != nil:
		return problem(c, http.StatusInternalServerError, "background_not_stored")
	}
	return c.JSON(http.StatusCreated, background)
}

// Background は、画像そのものを返す。
//
// **型は中身から決まったものを名乗る。** 送られてきたときに名乗られた型では
// ない——それはこのバイト列について何も保証しない。X-Content-Type-Options は
// Security.Middleware が全応答に付けているので、ここで名乗った型より先へ
// ブラウザが推測することはない。
func (h ConfigHandlers) Background(c *echo.Context) error {
	contents, mediaType, err := h.Service.BackgroundContents(c.Param("name"))
	if errors.Is(err, application.ErrUnknownBackground) {
		return problem(c, http.StatusNotFound, "unknown_background")
	}
	if err != nil {
		return problem(c, http.StatusInternalServerError, "backgrounds_unreadable")
	}
	return c.Blob(http.StatusOK, mediaType, contents)
}

func (h ConfigHandlers) DeleteBackground(c *echo.Context) error {
	err := h.Service.RemoveBackground(c.Param("name"))
	if errors.Is(err, application.ErrUnknownBackground) {
		return problem(c, http.StatusNotFound, "unknown_background")
	}
	if err != nil {
		return problem(c, http.StatusInternalServerError, "background_not_removed")
	}
	return c.NoContent(http.StatusNoContent)
}
