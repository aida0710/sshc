package httpserver

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
)

// forwardedRoute は、実行先のエンジンへそのまま取り次ぐ操作1つである。path は /api/v1 のあとで、
// このマシンの同じ操作と同じ形である。
type forwardedRoute struct {
	method string
	path   string
}

// forwardedRoutes は、リモートセッションを選んでいる間、画面が実行先のものとして出す操作である。
// ここに無い操作は取り次がない。同期、更新、設定、このマシンのセッション、スニペットは、実行先を
// 選んでいてもこのマシンのものなので、足さない（TestWhatStaysThisMachinesIsNeverForwarded）。
//
// 足せるのは、短い JSON を送ってすぐ答える操作だけである（remoteengine.ForwardAPI の本文と時間の上限）。
// 契約（api/openapi.yaml）には、このマシンの操作だけを書く。取り次ぐ操作は、同じ要求と応答に、
// 実行先へ届かないときの 502 が加わるだけである。
var forwardedRoutes = []forwardedRoute{
	// 確認のトークン。実行先の操作を確認するトークンは、実行先のエンジンが出して確かめる。
	{http.MethodPost, "/actions"},
	// Home の接続先の一覧と最近の接続。
	{http.MethodGet, "/config/overview"},
	{http.MethodGet, "/connections/recent"},
	// Known Hosts。
	{http.MethodGet, "/known-hosts"},
	{http.MethodPost, "/known-hosts/delete"},
	{http.MethodPost, "/known-hosts/scan"},
	{http.MethodPost, "/known-hosts/add"},
}

// registerForwardedRoutes は、forwardedRoutes を /api/v1/engines/:alias の下に登録する。
func registerForwardedRoutes(engine *echo.Echo, handlers RemoteEngineHandlers) {
	for _, route := range forwardedRoutes {
		engine.Add(route.method, enginesPrefix+"/:alias"+route.path, handlers.Forward)
	}
}

// Forward は、実行先のエンジンの、ターミナル以外の操作を取り次ぐ。このマシンのセッションと
// CSRF は、ここへ届く前にこのマシンの規則で確かめてある。
func (h RemoteEngineHandlers) Forward(c *echo.Context) error {
	alias, ok := aliasParam(c)
	if !ok {
		return problem(c, http.StatusBadRequest, "unsafe_alias")
	}
	remotePath := "/api/v1" + strings.TrimPrefix(c.Request().URL.Path, enginesPrefix+"/"+alias)
	if err := h.Engines.ForwardAPI(c.Response(), c.Request(), alias, remotePath); err != nil {
		return remoteEngineProblem(c, err)
	}
	return nil
}
