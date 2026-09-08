package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/application"
)

// 端末の背景画像。
//
// 名前を決めるのはサーバーである。送られてくるのは希望と中身だけで、
// 実際の表記と型は応答が返す。送られた表記をそのままファイル名にすれば、
// `../` も隠しファイルも拡張子の詐称もそこから入る。
//
// 画像本体は暗号化スナップショットに含める（remotesync.Collect）。Android は
// サンドボックス外を参照できないため、同期で画像を受け取る。

func registerBackgroundRoutes(engine *echo.Echo, handlers ConfigHandlers) {
	engine.GET("/api/v1/terminal/backgrounds", handlers.Backgrounds)
	engine.POST("/api/v1/terminal/backgrounds", handlers.AddBackground)
	engine.PUT("/api/v1/terminal/backgrounds/capacity", handlers.SetBackgroundCapacity)
	engine.GET("/api/v1/terminal/backgrounds/:name", handlers.Background)
	engine.PATCH("/api/v1/terminal/backgrounds/:name", handlers.RenameBackground)
	engine.DELETE("/api/v1/terminal/backgrounds/:name", handlers.DeleteBackground)
}

type backgroundListResponse struct {
	Backgrounds    []api.TerminalBackground `json:"backgrounds"`
	UsedBytes      int                      `json:"usedBytes"`
	CapacityBytes  int64                    `json:"capacityBytes"`
	RemainingBytes int                      `json:"remainingBytes"`
}

func terminalBackground(background application.Background) api.TerminalBackground {
	return api.TerminalBackground{Name: background.Name, Bytes: background.Bytes, Type: background.Type}
}

// backgroundList は、置いてある画像と、あと何バイト置けるかを返す。
//
// 残りを数えるのはこちらである。画面が上限を書き写すと、上限を変えた日に
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
	capacity := int64(h.Service.BackgroundCapacityMiB()) << 20
	remaining := capacity - int64(used)
	if remaining < 0 {
		remaining = 0
	}
	response := make([]api.TerminalBackground, 0, len(backgrounds))
	for _, background := range backgrounds {
		response = append(response, terminalBackground(background))
	}
	return c.JSON(http.StatusOK, backgroundListResponse{
		Backgrounds: response, UsedBytes: used, CapacityBytes: capacity, RemainingBytes: int(remaining),
	})
}

func (h ConfigHandlers) AddBackground(c *echo.Context) error {
	body := c.Request().Body
	if body == nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	// 1 バイト余分に読む。ちょうど上限で切ると、超えていることと
	// ちょうど収まっていることが見分けられない。
	limit := int64(h.Service.BackgroundCapacityMiB()) << 20
	contents, err := io.ReadAll(io.LimitReader(body, limit+1))
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
	return c.JSON(http.StatusCreated, terminalBackground(background))
}

func (h ConfigHandlers) SetBackgroundCapacity(c *echo.Context) error {
	var request struct {
		CapacityMiB int `json:"capacityMiB"`
	}
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if _, err := h.Service.SetBackgroundCapacityMiB(request.CapacityMiB); errors.Is(err, application.ErrBackgroundCapacity) {
		return problem(c, http.StatusBadRequest, "background_capacity_out_of_range")
	} else if err != nil {
		return serviceProblem(c, err)
	}
	return h.Backgrounds(c)
}

// Background は、画像そのものを返す。
//
// 型は中身から決まったものを名乗る。送られてきたときに名乗られた型では
// ない。それはこのバイト列について何も保証しない。X-Content-Type-Options は
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

func (h ConfigHandlers) RenameBackground(c *echo.Context) error {
	var request api.RenameTerminalBackgroundRequest
	decoder := json.NewDecoder(io.LimitReader(c.Request().Body, 1025))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || strings.TrimSpace(request.Name) == "" || len(request.Name) > 128 {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	background, err := h.Service.RenameBackground(c.Param("name"), request.Name)
	switch {
	case errors.Is(err, application.ErrUnknownBackground):
		return problem(c, http.StatusNotFound, "unknown_background")
	case errors.Is(err, application.ErrBackgroundAlreadyExists):
		return problem(c, http.StatusConflict, "background_already_exists")
	case errors.Is(err, application.ErrNotAnImage):
		return problem(c, http.StatusBadRequest, "not_an_image")
	case err != nil:
		return problem(c, http.StatusInternalServerError, "background_not_renamed")
	}
	return c.JSON(http.StatusOK, terminalBackground(background))
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
