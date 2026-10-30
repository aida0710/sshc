package httpserver

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/application"
)

// ConfigHandlers は、configuration・metadata・history の各エンドポイントを提供する。
// すべてのルートは same-origin であり、セッションで認証され、
// 変更操作については Security.Middleware が強制する CSRF ヘッダーの背後にある。
type ConfigHandlers struct {
	Service *application.Service
	// Keys は group 操作が必要とするインベントリを供給する。group の rename は
	// その鍵を移動させることを意味し、それらを指す IdentityFile をすべて書き換える。
	Keys KeyService
}

// historyList は、履歴の応答である。
//
// applicationの値を詰め替えずにそのまま載せる。重複する生成型は作らず、形の一致は
// acceptanceとwire contract testでOpenAPIへ直接照合する。
type historyList struct {
	Entries []application.HistoryEntry `json:"entries"`
}

// registerConfigRoutes は、各エンドポイントを Echo インスタンスに配線する。
func registerConfigRoutes(engine *echo.Echo, handlers ConfigHandlers) {
	engine.GET("/api/v1/config/overview", handlers.Overview)
	engine.GET("/api/v1/config/host", handlers.Host)
	engine.GET("/api/v1/config/file", handlers.File)
	engine.POST("/api/v1/config/preview", handlers.Preview)
	engine.POST("/api/v1/config/save", handlers.Save)
	engine.POST("/api/v1/config/groups/rename", handlers.RenameGroup)
	engine.POST("/api/v1/config/groups/delete", handlers.DeleteGroup)
	engine.GET("/api/v1/metadata", handlers.Metadata)
	engine.PUT("/api/v1/metadata/shortcuts", handlers.SetShortcuts)
	registerSettingsRoutes(engine, handlers)
	registerBackgroundRoutes(engine, handlers)
	engine.GET("/api/v1/history", handlers.History)
	engine.POST("/api/v1/history/restore", handlers.Restore)
	engine.POST("/api/v1/history/recover", handlers.Recover)
}

func (h ConfigHandlers) Overview(c *echo.Context) error {
	overview, err := h.Service.Overview()
	if err != nil {
		return serviceProblem(c, err)
	}
	return c.JSON(http.StatusOK, overview)
}

func (h ConfigHandlers) Host(c *echo.Context) error {
	query := c.Request().URL.Query()
	path, alias := query.Get("path"), query.Get("alias")
	if err := validatePathParameter(path); err != nil {
		return serviceProblem(c, err)
	}
	if err := validateHostBlockAlias(alias); err != nil {
		return serviceProblem(c, err)
	}
	detail, err := h.Service.HostDetail(path, alias)
	if err != nil {
		return serviceProblem(c, err)
	}
	return c.JSON(http.StatusOK, detail)
}

func (h ConfigHandlers) File(c *echo.Context) error {
	path := c.Request().URL.Query().Get("path")
	if err := validatePathParameter(path); err != nil {
		return serviceProblem(c, err)
	}
	contents, err := h.Service.FileContents(path)
	if err != nil {
		return serviceProblem(c, err)
	}
	return c.JSON(http.StatusOK, contents)
}

func (h ConfigHandlers) Preview(c *echo.Context) error {
	request, err := h.decodeEdit(c)
	if err != nil {
		return serviceProblem(c, err)
	}
	preview, err := h.Service.Preview(request)
	if err != nil {
		return serviceProblem(c, err)
	}
	return c.JSON(http.StatusOK, preview)
}

func (h ConfigHandlers) Save(c *echo.Context) error {
	request, err := h.decodeEdit(c)
	if err != nil {
		return serviceProblem(c, err)
	}
	result, err := h.Service.SaveWithSecrets(request)
	if err != nil {
		return serviceProblem(c, err)
	}
	return c.JSON(http.StatusOK, result)
}

// RenameGroup は、group ディレクトリと、それを指すすべてのものをリネームする。
// 鍵のパスフレーズの割り当ても、application が同じトランザクションで移す。
func (h ConfigHandlers) RenameGroup(c *echo.Context) error {
	var request api.GroupRenameRequest
	if err := decodeJSON(c, &request); err != nil {
		return serviceProblem(c, err)
	}
	inventory, err := h.Keys.Inventory()
	if err != nil {
		return unexpectedProblem(c, "inventory_failed", err)
	}
	result, err := h.Service.RenameGroup(inventory, request.From, request.To)
	if err != nil {
		return serviceProblem(c, err)
	}
	return c.JSON(http.StatusOK, result)
}

// DeleteGroup は、group を削除し、その接続を移動する。
func (h ConfigHandlers) DeleteGroup(c *echo.Context) error {
	var request api.GroupDeleteRequest
	if err := decodeJSON(c, &request); err != nil {
		return serviceProblem(c, err)
	}
	inventory, err := h.Keys.Inventory()
	if err != nil {
		return unexpectedProblem(c, "inventory_failed", err)
	}
	// Destination の空と未指定を分けない。契約では省略可能だが、どちらも
	// 「connections ディレクトリ自体へ移す」という同じ意味である。設定ファイルが
	// 削除されることは決してない。
	destination := ""
	if request.Destination != nil {
		destination = *request.Destination
	}
	result, err := h.Service.DeleteGroup(inventory, request.Name, destination)
	if err != nil {
		return serviceProblem(c, err)
	}
	return c.JSON(http.StatusOK, result)
}

func (h ConfigHandlers) decodeEdit(c *echo.Context) (application.EditRequest, error) {
	var request application.EditRequest
	if err := decodeJSON(c, &request); err != nil {
		return application.EditRequest{}, err
	}
	if err := validateEditRequest(request); err != nil {
		return application.EditRequest{}, err
	}
	return request, nil
}

func (h ConfigHandlers) Metadata(c *echo.Context) error {
	overview, err := h.Service.Overview()
	if err != nil {
		return serviceProblem(c, err)
	}
	return c.JSON(http.StatusOK, overview.Metadata)
}

func (h ConfigHandlers) History(c *echo.Context) error {
	entries, err := h.Service.History()
	if err != nil {
		return serviceProblem(c, err)
	}
	if entries == nil {
		entries = []application.HistoryEntry{}
	}
	return c.JSON(http.StatusOK, historyList{Entries: entries})
}

func (h ConfigHandlers) Restore(c *echo.Context) error {
	var request api.RestoreRequest
	if err := decodeJSON(c, &request); err != nil {
		return serviceProblem(c, err)
	}
	if err := validateIdentifier(request.TransactionId); err != nil {
		return serviceProblem(c, err)
	}
	if err := validatePathParameter(request.Path); err != nil {
		return serviceProblem(c, err)
	}
	result, err := h.Service.Restore(request.TransactionId, request.Path)
	if err != nil {
		return serviceProblem(c, err)
	}
	return c.JSON(http.StatusOK, result)
}

func (h ConfigHandlers) Recover(c *echo.Context) error {
	var request api.RecoverRequest
	if err := decodeJSON(c, &request); err != nil {
		return serviceProblem(c, err)
	}
	if err := validateIdentifier(request.TransactionId); err != nil {
		return serviceProblem(c, err)
	}
	if request.Action != "complete" && request.Action != "rollback" {
		return serviceProblem(c, errInvalidEdit)
	}
	if err := h.Service.Recover(request.TransactionId, request.Action); err != nil {
		return serviceProblem(c, err)
	}
	return c.JSON(http.StatusOK, api.RecoverResponse{Status: "ok"})
}

func (h ConfigHandlers) SetShortcuts(c *echo.Context) error {
	var request struct {
		Base    []application.ShortcutPreset `json:"base"`
		Presets []application.ShortcutPreset `json:"presets"`
	}
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if request.Base == nil || request.Presets == nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	result, err := h.Service.SetShortcutPresets(request.Base, request.Presets)
	if errors.Is(err, application.ErrShortcutConflict) {
		return problem(c, http.StatusConflict, "shortcut_presets_changed")
	}
	if errors.Is(err, application.ErrShortcutPresets) {
		return problem(c, http.StatusBadRequest, "invalid_shortcut_presets")
	}
	if err != nil {
		return serviceProblem(c, err)
	}
	return c.JSON(http.StatusOK, result.Settings())
}
