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
	// SSH の設定: Home と Connections の接続先、SSH Config のファイル、変更の履歴。
	{http.MethodGet, "/config/overview"},
	{http.MethodGet, "/config/host"},
	{http.MethodGet, "/config/file"},
	{http.MethodPost, "/config/preview"},
	{http.MethodPost, "/config/save"},
	{http.MethodPost, "/config/groups/rename"},
	{http.MethodPost, "/config/groups/delete"},
	{http.MethodGet, "/connections/recent"},
	{http.MethodPost, "/connections"},
	{http.MethodPatch, "/connections"},
	{http.MethodGet, "/history"},
	{http.MethodPost, "/history/restore"},
	{http.MethodPost, "/history/recover"},
	{http.MethodGet, "/history/pending/:id"},
	// SSH Keys とごみ箱、Remote Keys。
	{http.MethodGet, "/keys"},
	{http.MethodPost, "/keys"},
	{http.MethodGet, "/keys/algorithms"},
	{http.MethodPost, "/keys/hardware-command"},
	{http.MethodPost, "/keys/:keyId/passphrase"},
	{http.MethodPost, "/keys/:keyId/reveal"},
	{http.MethodGet, "/keys/:keyId/public"},
	{http.MethodPost, "/keys/:keyId/location"},
	{http.MethodPost, "/keys/:keyId/agent"},
	{http.MethodDelete, "/keys/:keyId/agent"},
	{http.MethodPost, "/keys/:keyId/trash"},
	{http.MethodGet, "/trash"},
	{http.MethodPost, "/trash/:entryId/restore"},
	{http.MethodDelete, "/trash/:entryId"},
	{http.MethodPost, "/remote-keys/plan"},
	{http.MethodPost, "/remote-keys/register"},
	// Vault。作ること、開け閉め、入れた値の操作まで。マスターパスワードの変更と、壊れた Vault の
	// 復旧・作り直しは取り次がない（実行先の画面でする）。
	{http.MethodGet, "/passwords"},
	{http.MethodPost, "/passwords/initialise"},
	{http.MethodPost, "/passwords/unlock"},
	{http.MethodPost, "/passwords/lock"},
	{http.MethodGet, "/passwords/:alias/eligibility"},
	{http.MethodGet, "/credentials"},
	{http.MethodPut, "/credentials/:kind/assign"},
	{http.MethodDelete, "/credentials/:kind/assign/:subject"},
	{http.MethodPut, "/credentials/:kind/:name"},
	{http.MethodPatch, "/credentials/:kind/:name"},
	{http.MethodPost, "/credentials/:kind/:name/reveal"},
	{http.MethodPost, "/credentials/totp/:name/codes"},
	{http.MethodDelete, "/credentials/:kind/:name"},
	// VPN。
	{http.MethodGet, "/vpn"},
	{http.MethodPost, "/vpn/profiles"},
	{http.MethodPut, "/vpn/profiles/:name"},
	{http.MethodDelete, "/vpn/profiles/:name"},
	{http.MethodPost, "/vpn/profiles/:name/rename"},
	{http.MethodPost, "/vpn/profiles/:name/reveal"},
	{http.MethodGet, "/vpn/profiles/:name/logs"},
	{http.MethodPost, "/vpn/profiles/:name/route"},
	{http.MethodDelete, "/vpn/profiles/:name/route"},
	{http.MethodPut, "/vpn/bindings"},
	// Diagnostics。
	{http.MethodPost, "/diagnostics/config"},
	{http.MethodPost, "/diagnostics/effective"},
	{http.MethodPost, "/diagnostics/reachability"},
	{http.MethodPost, "/diagnostics/authentication"},
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
