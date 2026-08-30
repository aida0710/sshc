package httpserver

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/application"
	"sshc/internal/platform"
	"sshc/internal/secret"
)

// ConfigHandlers は、configuration・metadata・history の各エンドポイントを提供する。
// すべてのルートは same-origin であり、セッションで認証され、
// 変更操作については Security.Middleware が強制する CSRF ヘッダーの背後にある。
type ConfigHandlers struct {
	Service *application.Service
	Secrets *secret.Service
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
	engine.PUT("/api/v1/metadata/terminal", handlers.SetTerminal)
	engine.PUT("/api/v1/metadata/engine", handlers.SetEngine)
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
	if err := validateAliasParameter(alias); err != nil {
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
	result, err := h.Service.Save(request)
	if err != nil {
		return serviceProblem(c, err)
	}
	return c.JSON(http.StatusOK, result)
}

// RenameGroup は、group ディレクトリと、それを指すすべてのものをリネームする。
func (h ConfigHandlers) RenameGroup(c *echo.Context) error {
	var request api.GroupRenameRequest
	if err := decodeJSON(c, &request); err != nil {
		return serviceProblem(c, err)
	}
	inventory, err := h.Keys.Inventory()
	if err != nil {
		return problem(c, http.StatusInternalServerError, "inventory_failed")
	}
	result, err := h.Service.RenameGroup(inventory, request.From, request.To)
	if err != nil {
		return serviceProblem(c, err)
	}
	if h.Secrets != nil {
		if err := h.Secrets.RelocateKeyPassphrases(keyRelocationMap(result.KeyRelocations)); err != nil {
			return serviceProblem(c, err)
		}
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
		return problem(c, http.StatusInternalServerError, "inventory_failed")
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
	if h.Secrets != nil {
		if err := h.Secrets.RelocateKeyPassphrases(keyRelocationMap(result.KeyRelocations)); err != nil {
			return serviceProblem(c, err)
		}
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

// SetTerminal は、ローカルシェルの開始位置を書き戻す。
//
// 通らない指定はここで断る。保存を受け入れておいて、次に端末を開いた
// ときに初めて失敗させると、設定画面と失敗の現れる場所が離れる。
func (h ConfigHandlers) SetTerminal(c *echo.Context) error {
	var request api.TerminalSettings
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	settings := application.TerminalSettings{}
	if request.StartDirectory != nil {
		settings.StartDirectory = *request.StartDirectory
	}
	if request.MaxSessions != nil {
		settings.MaxSessions = *request.MaxSessions
	}
	if request.ScrollbackBytes != nil {
		settings.ScrollbackBytes = *request.ScrollbackBytes
	}
	if request.BrowserScrollbackLines != nil {
		settings.BrowserScrollbackLines = *request.BrowserScrollbackLines
	}
	if request.FontSize != nil {
		settings.FontSize = *request.FontSize
	}
	if request.Verbosity != nil {
		settings.Verbosity = *request.Verbosity
	}
	// 0 は「繋ぎ直さない」であって「書かれていない」ではない。値で受けると、
	// 切ったつもりのユーザーが既定へ戻される。
	settings.Reconnect = request.Reconnect
	settings.CopyOnSelect = request.CopyOnSelect
	settings.RightClickPaste = request.RightClickPaste
	if request.Osc52 != nil {
		settings.OSC52 = *request.Osc52
	}
	if request.JisYenBackslash != nil {
		settings.JISYenBackslash = *request.JisYenBackslash
	}
	if request.LocalShellProfile != nil {
		settings.LocalShellProfile = *request.LocalShellProfile
	}
	if request.Appearance != nil {
		// 未知の配色名は保存できるようにする。UI 側は未知の名前を既定値として扱うため、
		// 配色名を変更しても設定画面を利用できる。
		if request.Appearance.Palette != nil {
			settings.Appearance.Palette = *request.Appearance.Palette
		}
		if request.Appearance.Font != nil {
			settings.Appearance.Font = *request.Appearance.Font
		}
		if request.Appearance.Background != nil {
			settings.Appearance.Background = *request.Appearance.Background
		}
		// 0 は「かぶせない」であって「選んでいない」ではない。書かれて
		// いないときだけ、上の段の選択と既定に譲る。
		settings.Appearance.BackgroundTint = request.Appearance.BackgroundTint
	}
	result, err := h.Service.SetTerminalSettings(settings)
	switch {
	case errors.Is(err, platform.ErrDirectoryRelative), errors.Is(err, platform.ErrDirectoryUser):
		return problem(c, http.StatusBadRequest, "start_directory_unusable")
	case errors.Is(err, application.ErrStartDirectoryMissing):
		return problem(c, http.StatusBadRequest, "start_directory_missing")
	case errors.Is(err, application.ErrStartDirectoryNotADirectory):
		return problem(c, http.StatusBadRequest, "start_directory_not_a_directory")
	// 範囲の外は書き込みで断る。読み取りが既定へ戻すのとは対称ではない
	// これはこのアプリケーション自身の操作であり、断ればユーザーが直せる。
	case errors.Is(err, application.ErrMetadataTerminal):
		return problem(c, http.StatusBadRequest, "terminal_limits_out_of_range")
	case err != nil:
		return serviceProblem(c, err)
	}
	return c.JSON(http.StatusOK, result)
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

// SetEngine は、engine そのものの設定を書き戻す。
//
// 受け口の番号は起動時にしか読まれない。変えても、次に engine を起動するまで
// 効かない。画面はそう言う。
func (h ConfigHandlers) SetEngine(c *echo.Context) error {
	var request api.EngineSettings
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	settings := application.EngineSettings{}
	if request.Port != nil {
		// 範囲はここで断る。通せば、断るのは次の起動の bind であり、
		// そのとき画面はもう閉じている。
		if *request.Port < 1024 || *request.Port > 65535 {
			return problem(c, http.StatusBadRequest, "port_out_of_range")
		}
		settings.Port = *request.Port
	}
	if request.VaultAutoLock != nil {
		chosen := request.VaultAutoLock
		settings.VaultAutoLock = &application.VaultAutoLock{Mode: string(chosen.Mode)}
		switch chosen.Mode {
		case api.Restart:
			if chosen.Value != nil || chosen.Unit != nil {
				return problem(c, http.StatusBadRequest, "invalid_vault_auto_lock")
			}
		case api.Idle:
			if chosen.Value == nil || *chosen.Value < 1 || *chosen.Value > 999 || chosen.Unit == nil ||
				(*chosen.Unit != api.Minutes && *chosen.Unit != api.Hours) {
				return problem(c, http.StatusBadRequest, "invalid_vault_auto_lock")
			}
			settings.VaultAutoLock.Value = *chosen.Value
			settings.VaultAutoLock.Unit = string(*chosen.Unit)
		default:
			return problem(c, http.StatusBadRequest, "invalid_vault_auto_lock")
		}
	}
	result, err := h.Service.SetEngineSettings(settings)
	if err != nil {
		return serviceProblem(c, err)
	}
	if h.Secrets != nil {
		h.Secrets.SetIdleTimeout(settings.VaultIdleTimeout(secret.IdleTimeout))
	}
	return c.JSON(http.StatusOK, result)
}
