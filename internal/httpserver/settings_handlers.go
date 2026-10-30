package httpserver

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/application"
	"sshc/internal/platform"
)

// 設定（全体の設定・プリセット・マシン）の読み書き。中身は metadata とマシンごとのファイルの形の
// まま受け取り、範囲は application が確かめる。
func registerSettingsRoutes(engine *echo.Echo, handlers ConfigHandlers) {
	engine.GET("/api/v1/settings", handlers.Settings)
	engine.PUT("/api/v1/settings/global", handlers.SetGlobalSettings)
	engine.PUT("/api/v1/settings/presets", handlers.SetPresets)
	engine.PUT("/api/v1/settings/presets/:id", handlers.SetPreset)
	engine.PUT("/api/v1/settings/machines/:id", handlers.SetMachineSettings)
	engine.DELETE("/api/v1/settings/machines/:id", handlers.RemoveMachine)
	engine.PUT("/api/v1/settings/state", handlers.SetMachineState)
}

// Settings は、設定の画面が読むもの全部を返す。
func (h ConfigHandlers) Settings(c *echo.Context) error {
	document, err := h.Service.Settings()
	if err != nil {
		return serviceProblem(c, err)
	}
	return c.JSON(http.StatusOK, document)
}

func (h ConfigHandlers) SetGlobalSettings(c *echo.Context) error {
	var request struct {
		Base     *application.GlobalSettings `json:"base"`
		Settings *application.GlobalSettings `json:"settings"`
	}
	if err := decodeJSON(c, &request); err != nil || request.Base == nil || request.Settings == nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	result, err := h.Service.SetGlobalSettings(*request.Base, *request.Settings)
	return settingsSaved(c, result, err)
}

// SetPresets は、プリセットの一覧を置き換える（作る・名前を変える・消す・並べ替える）。
func (h ConfigHandlers) SetPresets(c *echo.Context) error {
	var request struct {
		Base    []application.SettingsPreset `json:"base"`
		Presets []application.SettingsPreset `json:"presets"`
	}
	if err := decodeJSON(c, &request); err != nil || request.Base == nil || request.Presets == nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	result, err := h.Service.SetPresets(request.Base, request.Presets)
	return settingsSaved(c, result, err)
}

// SetPreset は、1つのプリセットの中身を置き換える。
func (h ConfigHandlers) SetPreset(c *echo.Context) error {
	var request struct {
		Base   *application.SettingsPreset `json:"base"`
		Preset *application.SettingsPreset `json:"preset"`
	}
	if err := decodeJSON(c, &request); err != nil || request.Base == nil || request.Preset == nil || request.Preset.ID != c.Param("id") {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	result, err := h.Service.SetPreset(*request.Base, *request.Preset)
	return settingsSaved(c, result, err)
}

// SetMachineSettings は、1台のマシンの設定（名前・使うプリセット・ポート）を置き換える。
func (h ConfigHandlers) SetMachineSettings(c *echo.Context) error {
	var request struct {
		Base     *application.MachineSettings `json:"base"`
		Settings *application.MachineSettings `json:"settings"`
	}
	if err := decodeJSON(c, &request); err != nil || request.Base == nil || request.Settings == nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	result, err := h.Service.SetMachineSettings(c.Param("id"), *request.Base, *request.Settings)
	return settingsSaved(c, result, err)
}

// RemoveMachine は、使わなくなったマシンを一覧から外す。
func (h ConfigHandlers) RemoveMachine(c *echo.Context) error {
	if err := h.Service.RemoveMachine(c.Param("id")); err != nil {
		return settingsProblem(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// SetMachineState は、このマシンの今の状態のうち、送られた分を変える。画面の並びは書くのを
// まとめて後で行うので、すぐに返す。転送の処理を止めているかは書いてから転送キューに当てる。
func (h ConfigHandlers) SetMachineState(c *echo.Context) error {
	var request struct {
		Interface        map[string]string `json:"interface"`
		TransfersStopped *bool             `json:"transfersStopped"`
	}
	if err := decodeJSON(c, &request); err != nil || (request.Interface == nil && request.TransfersStopped == nil) {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if request.Interface != nil {
		if err := h.Service.SetInterfaceState(request.Interface); err != nil {
			return settingsProblem(c, err)
		}
	}
	if request.TransfersStopped != nil {
		if err := h.Service.SetTransfersStopped(*request.TransfersStopped); err != nil {
			return settingsProblem(c, err)
		}
		// 今の状態は commit を通らないので、転送キューの読み直しをここで走らせる。
		h.Service.SettingsMayHaveChanged()
	}
	return c.NoContent(http.StatusNoContent)
}

func settingsSaved(c *echo.Context, result application.SaveResult, err error) error {
	if err != nil {
		return settingsProblem(c, err)
	}
	return c.JSON(http.StatusOK, result.Settings())
}

// settingsProblem は、設定の保存で断った理由を、画面が直し方を言える code にする。
func settingsProblem(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, application.ErrSettingsChanged):
		return problem(c, http.StatusConflict, "settings_changed")
	case errors.Is(err, application.ErrThisMachine):
		return problem(c, http.StatusConflict, "this_machine")
	case errors.Is(err, application.ErrUnknownPreset), errors.Is(err, application.ErrUnknownMachine):
		return problem(c, http.StatusNotFound, "not_found")
	// 範囲の外は保存で断る。通せば、断るのは次の起動の bind であり、そのとき画面はもう閉じている。
	case errors.Is(err, application.ErrEnginePort):
		return problem(c, http.StatusBadRequest, "port_out_of_range")
	case errors.Is(err, platform.ErrDirectoryRelative), errors.Is(err, platform.ErrDirectoryUser):
		return problem(c, http.StatusBadRequest, "start_directory_unusable")
	case errors.Is(err, application.ErrStartDirectoryMissing):
		return problem(c, http.StatusBadRequest, "start_directory_missing")
	case errors.Is(err, application.ErrStartDirectoryNotADirectory):
		return problem(c, http.StatusBadRequest, "start_directory_not_a_directory")
	case errors.Is(err, application.ErrPresets), errors.Is(err, application.ErrMetadataTerminal),
		errors.Is(err, application.ErrMetadataAppearance), errors.Is(err, application.ErrVaultAutoLock),
		errors.Is(err, application.ErrMachineSettings), errors.Is(err, application.ErrMachineState):
		return problem(c, http.StatusBadRequest, "invalid_settings")
	default:
		return serviceProblem(c, err)
	}
}
