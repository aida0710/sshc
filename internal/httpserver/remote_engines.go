package httpserver

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/remoteengine"
	"sshc/internal/validate"
)

// RemoteEngineDirectory は、実行先に足した接続先のエンジンを持ち、そのターミナルを取り次ぐ。
type RemoteEngineDirectory interface {
	RemoteEngineOpener
	Engines() ([]string, error)
	AddEngine(ctx context.Context, alias string) error
	RemoveEngine(alias string) error
	EngineStatus(ctx context.Context, alias string) (remoteengine.Status, error)
	ControlEngine(ctx context.Context, alias string, action remoteengine.EngineAction) (remoteengine.ActionResult, error)
	UnlockVault(ctx context.Context, alias, passphrase string) error
	ForwardTerminalAPI(w http.ResponseWriter, r *http.Request, alias, remotePath string) error
	ForwardTerminalStream(w http.ResponseWriter, r *http.Request, alias string) error
}

// RemoteEngineHandlers は、画面が実行先を選ぶための受け口と、選んだ実行先のターミナルの
// 受け口である。
type RemoteEngineHandlers struct {
	Engines RemoteEngineDirectory
	// ExpectedOrigin は、取り次ぐ WebSocket で完全一致を求めるこのマシンの Origin である。
	ExpectedOrigin string
}

// enginesPrefix は、実行先のエンジンの受け口の頭である。:alias のあとの /terminal… は、
// 接続先のエンジンの /api/v1/terminal… へそのまま取り次ぐ。
const enginesPrefix = "/api/v1/engines"

// registerRemoteEngineRoutes は、実行先のエンジンの受け口を登録する。取り次ぐのは、画面が
// 使うターミナルの操作だけである。ポート転送、一括送信、制御用の読み取りは、今の段階では
// 取り次がない（画面が出さない）。経路は契約の検査がソースから読むので、一本ずつ書く。
func registerRemoteEngineRoutes(engine *echo.Echo, handlers RemoteEngineHandlers) {
	engine.GET("/api/v1/engines", handlers.List)
	engine.POST("/api/v1/engines", handlers.Add)
	engine.DELETE("/api/v1/engines/:alias", handlers.Remove)
	engine.GET("/api/v1/engines/:alias/status", handlers.Status)
	engine.POST("/api/v1/engines/:alias/open", handlers.Open)
	engine.POST("/api/v1/engines/:alias/control", handlers.Control)
	engine.POST("/api/v1/engines/:alias/vault/unlock", handlers.UnlockVault)
	engine.GET("/api/v1/engines/:alias/terminal/sessions", handlers.ForwardTerminal)
	engine.POST("/api/v1/engines/:alias/terminal/sessions", handlers.ForwardTerminal)
	engine.GET("/api/v1/engines/:alias/terminal/shell-profiles", handlers.ForwardTerminal)
	engine.POST("/api/v1/engines/:alias/terminal/sessions/:id/stream", handlers.ForwardTerminal)
	engine.POST("/api/v1/engines/:alias/terminal/sessions/:id/reconnect", handlers.ForwardTerminal)
	engine.POST("/api/v1/engines/:alias/terminal/sessions/:id/reconnect/stop", handlers.ForwardTerminal)
	engine.PUT("/api/v1/engines/:alias/terminal/sessions/:id/title", handlers.ForwardTerminal)
	engine.PUT("/api/v1/engines/:alias/terminal/sessions/:id/workspace", handlers.ForwardTerminal)
	engine.GET("/api/v1/engines/:alias/terminal/sessions/:id/directory", handlers.ForwardTerminal)
	engine.DELETE("/api/v1/engines/:alias/terminal/sessions/:id", handlers.ForwardTerminal)
	engine.GET("/api/v1/engines/:alias/terminal/workspaces", handlers.ForwardTerminal)
	engine.POST("/api/v1/engines/:alias/terminal/workspaces", handlers.ForwardTerminal)
	engine.PATCH("/api/v1/engines/:alias/terminal/workspaces/:id", handlers.ForwardTerminal)
	engine.DELETE("/api/v1/engines/:alias/terminal/workspaces/:id", handlers.ForwardTerminal)
	engine.GET(remoteEngineStreamRoute, handlers.ForwardStream)
}

// remoteEngineStreamRoute は、実行先のエンジンのターミナルの入出力の WebSocket である。
// /api/ の外にあるが、Security がこのマシンのセッションと Vault のロックの解除を求める。
const remoteEngineStreamRoute = "/engines/:alias/terminal/stream"

func (h RemoteEngineHandlers) engineList() (api.RemoteEngineList, error) {
	aliases, err := h.Engines.Engines()
	if err != nil {
		return api.RemoteEngineList{}, err
	}
	engines := make([]api.RemoteEngine, 0, len(aliases))
	for _, alias := range aliases {
		engines = append(engines, api.RemoteEngine{Alias: alias})
	}
	return api.RemoteEngineList{Engines: engines}, nil
}

// aliasParam は、パスの :alias を、接続先として使ってよい alias として返す。
func aliasParam(c *echo.Context) (string, bool) {
	alias := c.Param("alias")
	return alias, validate.Alias(alias) == nil
}

func (h RemoteEngineHandlers) List(c *echo.Context) error {
	list, err := h.engineList()
	if err != nil {
		return remoteEngineProblem(c, err)
	}
	return c.JSON(http.StatusOK, list)
}

// Add は、接続先のエンジンを実行先に足す。接続でき、同じバージョンのエンジンが動いている
// ことを確かめてから足す。
func (h RemoteEngineHandlers) Add(c *echo.Context) error {
	var request api.AddRemoteEngineRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if err := validate.Alias(request.Alias); err != nil {
		return problem(c, http.StatusBadRequest, "unsafe_alias")
	}
	if err := h.Engines.AddEngine(c.Request().Context(), request.Alias); err != nil {
		return remoteEngineProblem(c, err)
	}
	return c.JSON(http.StatusCreated, api.RemoteEngine{Alias: request.Alias})
}

// Remove は、実行先からエンジンを外す。接続先のエンジンとそのターミナルは止めない。
func (h RemoteEngineHandlers) Remove(c *echo.Context) error {
	alias, ok := aliasParam(c)
	if !ok {
		return problem(c, http.StatusBadRequest, "unsafe_alias")
	}
	if err := h.Engines.RemoveEngine(alias); err != nil {
		return remoteEngineProblem(c, err)
	}
	list, err := h.engineList()
	if err != nil {
		return remoteEngineProblem(c, err)
	}
	return c.JSON(http.StatusOK, list)
}

func (h RemoteEngineHandlers) Status(c *echo.Context) error {
	alias, ok := aliasParam(c)
	if !ok {
		return problem(c, http.StatusBadRequest, "unsafe_alias")
	}
	status, err := h.Engines.EngineStatus(c.Request().Context(), alias)
	if err != nil {
		return remoteEngineProblem(c, err)
	}
	return c.JSON(http.StatusOK, api.RemoteEngineStatus{
		Alias: alias, Version: status.Version, Vault: status.Vault, Unlocked: status.Unlocked,
		Passwordless: status.Passwordless, Sessions: status.Sessions,
	})
}

// Open は、接続先のエンジンの画面を別のタブで開く URL を返す（sshc open <alias> と同じ）。
func (h RemoteEngineHandlers) Open(c *echo.Context) error {
	alias, ok := aliasParam(c)
	if !ok {
		return problem(c, http.StatusBadRequest, "unsafe_alias")
	}
	opened, err := h.Engines.Open(c.Request().Context(), alias)
	if err != nil {
		return remoteEngineProblem(c, err)
	}
	return c.JSON(http.StatusOK, api.RemoteEngineEntrance{Url: opened})
}

// Control は、実行先のエンジンを止める・起動し直す・更新する。どれも接続先のターミナルを
// すべて止めるので、画面が尋ねてから送る。
func (h RemoteEngineHandlers) Control(c *echo.Context) error {
	alias, ok := aliasParam(c)
	if !ok {
		return problem(c, http.StatusBadRequest, "unsafe_alias")
	}
	var request api.RemoteEngineActionRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	result, err := h.Engines.ControlEngine(c.Request().Context(), alias, remoteengine.EngineAction(request.Action))
	if errors.Is(err, remoteengine.ErrUnknownAction) {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if err != nil {
		return remoteEngineProblem(c, err)
	}
	return c.JSON(http.StatusOK, api.RemoteEngineActionResult{Succeeded: result.Succeeded, Output: result.Output})
}

// UnlockVault は、実行先のエンジンの Vault のロックを解除する。パスワードは接続先へ
// 渡すだけで、このマシンには残さない。
func (h RemoteEngineHandlers) UnlockVault(c *echo.Context) error {
	alias, ok := aliasParam(c)
	if !ok {
		return problem(c, http.StatusBadRequest, "unsafe_alias")
	}
	var request api.PassphraseRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if err := h.Engines.UnlockVault(c.Request().Context(), alias, request.Passphrase); err != nil {
		return remoteEngineProblem(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// ForwardTerminal は、実行先のエンジンのターミナルの操作を取り次ぐ。このマシンの
// セッションと CSRF は、ここへ届く前にこのマシンの規則で確かめてある。
func (h RemoteEngineHandlers) ForwardTerminal(c *echo.Context) error {
	alias, ok := aliasParam(c)
	if !ok {
		return problem(c, http.StatusBadRequest, "unsafe_alias")
	}
	local := c.Request().URL.Path
	remotePath := "/api/v1/terminal" + strings.TrimPrefix(local, enginesPrefix+"/"+alias+"/terminal")
	if err := h.Engines.ForwardTerminalAPI(c.Response(), c.Request(), alias, remotePath); err != nil {
		return remoteEngineProblem(c, err)
	}
	return nil
}

// ForwardStream は、実行先のエンジンのターミナルの入出力を取り次ぐ。このマシンの
// /terminal/stream と同じく、Origin の完全一致を求める。このマシンのセッションと Vault の
// ロックの解除は Security が確かめ終えている。ターミナルへの認可は、接続先のエンジンが
// 発行した使い捨てのチケットで、接続先が確かめる。
func (h RemoteEngineHandlers) ForwardStream(c *echo.Context) error {
	request := c.Request()
	if request.Header.Get(echo.HeaderOrigin) != h.ExpectedOrigin {
		return c.NoContent(http.StatusForbidden)
	}
	if site := request.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" {
		return c.NoContent(http.StatusForbidden)
	}
	alias, ok := aliasParam(c)
	if !ok {
		return c.NoContent(http.StatusForbidden)
	}
	if err := h.Engines.ForwardTerminalStream(c.Response(), request, alias); err != nil {
		return remoteEngineProblem(c, err)
	}
	return nil
}
