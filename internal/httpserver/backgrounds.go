package httpserver

import (
	"errors"
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

const backgroundsRoute = "/api/v1/terminal/backgrounds"

// MaxBackgroundUploadBodyCeiling は、背景画像を追加する本文の入口の上限である。
// 1 枚の絶対上限と同じにする。/api/ 共通の上限のままだと、利用者が容量を
// 増やしても 2 MiB を超える画像を置けない。設定された容量は AddBackground が押さえる。
const MaxBackgroundUploadBodyCeiling = int64(application.MaxBackgroundBytes)

// maxBackgroundRenameBody は、背景の改名のボディの上限である。名前ひとつ
// （application.MaxBackgroundNameLength）を JSON の文字列にしても十分に収まる。
const maxBackgroundRenameBody = 1 << 10

func registerBackgroundRoutes(engine *echo.Echo, handlers ConfigHandlers) {
	engine.GET(backgroundsRoute, handlers.Backgrounds)
	engine.POST(backgroundsRoute, handlers.AddBackground)
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

// Backgroundsは、置いてある画像と、あと何バイト置けるかを返す。
//
// 残りを数えるのはサーバーである（application.Service.BackgroundUsage）。画面が上限を
// 書き写すと、上限を変えた日に画面だけが古い数を信じる。
func (h ConfigHandlers) Backgrounds(c *echo.Context) error {
	backgrounds, err := h.Service.Backgrounds()
	if err != nil {
		return unexpectedProblem(c, "backgrounds_unreadable", err)
	}
	usage := h.Service.BackgroundUsage(backgrounds)
	response := make([]api.TerminalBackground, 0, len(backgrounds))
	for _, background := range backgrounds {
		response = append(response, terminalBackground(background))
	}
	return c.JSON(http.StatusOK, backgroundListResponse{
		Backgrounds:    response,
		UsedBytes:      int(usage.UsedBytes),
		CapacityBytes:  usage.CapacityBytes,
		RemainingBytes: int(usage.RemainingBytes),
	})
}

func (h ConfigHandlers) AddBackground(c *echo.Context) error {
	body := c.Request().Body
	if body == nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	background, err := h.Service.AddBackground(c.QueryParam("name"), body)
	var overCeiling *http.MaxBytesError
	switch {
	case errors.As(err, &overCeiling):
		// 長さを宣言しない本文が1枚の絶対上限を超えた。宣言した本文ならmiddlewareが先に
		// 断っている。MaxBytesErrorは本文の読み取りの失敗としてErrBackgroundUnreadableに
		// 包まれて届くので、このcaseをErrBackgroundUnreadableより先に置く。
		return problem(c, http.StatusRequestEntityTooLarge, "background_too_large")
	case errors.Is(err, application.ErrBackgroundUnreadable):
		return problem(c, http.StatusBadRequest, "invalid_request")
	case errors.Is(err, application.ErrBackgroundTooLarge):
		return problem(c, http.StatusRequestEntityTooLarge, "background_too_large")
	case errors.Is(err, application.ErrBackgroundsFull):
		return problem(c, http.StatusRequestEntityTooLarge, "backgrounds_full")
	case errors.Is(err, application.ErrNotAnImage):
		return problem(c, http.StatusBadRequest, "not_an_image")
	case err != nil:
		return unexpectedProblem(c, "background_not_stored", err)
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
		return unexpectedProblem(c, "backgrounds_unreadable", err)
	}
	return c.Blob(http.StatusOK, mediaType, contents)
}

func (h ConfigHandlers) RenameBackground(c *echo.Context) error {
	var request api.RenameTerminalBackgroundRequest
	if err := decodeJSONWithin(c, maxBackgroundRenameBody, &request); err != nil ||
		strings.TrimSpace(request.Name) == "" || len(request.Name) > application.MaxBackgroundNameLength {
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
		return unexpectedProblem(c, "background_not_renamed", err)
	}
	return c.JSON(http.StatusOK, terminalBackground(background))
}

func (h ConfigHandlers) DeleteBackground(c *echo.Context) error {
	err := h.Service.RemoveBackground(c.Param("name"))
	if errors.Is(err, application.ErrUnknownBackground) {
		return problem(c, http.StatusNotFound, "unknown_background")
	}
	if err != nil {
		return unexpectedProblem(c, "background_not_removed", err)
	}
	return c.NoContent(http.StatusNoContent)
}
