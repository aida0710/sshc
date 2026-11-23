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
	// slow は、実行先がよそのマシンの答えを待ってから答える操作かどうか。答えを長く待つ
	// （remoteengine.ForwardSlowAPI）。
	slow bool
	// file は、ファイルの中身を運ぶ操作かどうか。要求と答えの本文を、溜めずに流す
	// （remoteengine.ForwardFileAPI）。
	file bool
}

// forwardedMediaRoute は、実行先のファイルを <audio> と <video> が読む経路である。このマシンの
// sftpMediaRoute と同じく、CSRF のトークンの代わりに URL のチケットで確かめる。チケットは実行先の
// エンジンが出し、実行先が確かめる。このマシンは、セッションの cookie と Vault のロックの解除を求める。
const forwardedMediaRoute = enginesPrefix + "/:alias/sftp/media/:ticket"

// forwardedRoutes は、リモートセッションを選んでいる間、画面が実行先のものとして出す操作である。
// ここに無い操作は取り次がない。同期、更新、設定、このマシンのセッション、スニペットは、実行先を
// 選んでいてもこのマシンのものなので、足さない（TestWhatStaysThisMachinesIsNeverForwarded）。
//
// 短い JSON を送って JSON で答える操作は、そのまま足す（remoteengine.ForwardAPI の本文の上限に収まるもの）。
// すぐには答えない操作には slow を、ファイルの中身を運ぶ操作には file を付ける。
// 契約（api/openapi.yaml）には、このマシンの操作だけを書く。取り次ぐ操作は、同じ要求と応答に、
// 実行先へ届かないときの 502 が加わるだけである。
var forwardedRoutes = []forwardedRoute{
	// 確認のトークン。実行先の操作を確認するトークンは、実行先のエンジンが出して確かめる。
	{method: http.MethodPost, path: "/actions"},
	// SSH の設定: Home と Connections の接続先、SSH Config のファイル、変更の履歴。
	{method: http.MethodGet, path: "/config/overview"},
	{method: http.MethodGet, path: "/config/host"},
	{method: http.MethodGet, path: "/config/file"},
	{method: http.MethodPost, path: "/config/preview"},
	{method: http.MethodPost, path: "/config/save"},
	{method: http.MethodPost, path: "/config/groups/rename"},
	{method: http.MethodPost, path: "/config/groups/delete"},
	{method: http.MethodGet, path: "/connections/recent"},
	{method: http.MethodPost, path: "/connections"},
	{method: http.MethodPatch, path: "/connections"},
	{method: http.MethodGet, path: "/history"},
	{method: http.MethodPost, path: "/history/restore"},
	{method: http.MethodPost, path: "/history/recover"},
	{method: http.MethodGet, path: "/history/pending/:id"},
	// SSH Keys とごみ箱、Remote Keys。
	{method: http.MethodGet, path: "/keys"},
	{method: http.MethodPost, path: "/keys"},
	{method: http.MethodGet, path: "/keys/algorithms"},
	{method: http.MethodPost, path: "/keys/hardware-command"},
	{method: http.MethodPost, path: "/keys/:keyId/passphrase"},
	{method: http.MethodPost, path: "/keys/:keyId/reveal"},
	{method: http.MethodGet, path: "/keys/:keyId/public"},
	{method: http.MethodPost, path: "/keys/:keyId/location"},
	{method: http.MethodPost, path: "/keys/:keyId/agent"},
	{method: http.MethodDelete, path: "/keys/:keyId/agent"},
	{method: http.MethodPost, path: "/keys/:keyId/trash"},
	{method: http.MethodGet, path: "/trash"},
	{method: http.MethodPost, path: "/trash/:entryId/restore"},
	{method: http.MethodDelete, path: "/trash/:entryId"},
	{method: http.MethodPost, path: "/remote-keys/plan", slow: true},
	{method: http.MethodPost, path: "/remote-keys/register", slow: true},
	// Vault。作ること、開け閉め、入れた値の操作まで。マスターパスワードの変更と、壊れた Vault の
	// 復旧・作り直しは取り次がない（実行先の画面でする）。
	{method: http.MethodGet, path: "/passwords"},
	{method: http.MethodPost, path: "/passwords/initialise"},
	{method: http.MethodPost, path: "/passwords/unlock"},
	{method: http.MethodPost, path: "/passwords/lock"},
	{method: http.MethodGet, path: "/passwords/:alias/eligibility"},
	{method: http.MethodGet, path: "/credentials"},
	{method: http.MethodPut, path: "/credentials/:kind/assign"},
	{method: http.MethodDelete, path: "/credentials/:kind/assign/:subject"},
	{method: http.MethodPut, path: "/credentials/:kind/:name"},
	{method: http.MethodPatch, path: "/credentials/:kind/:name"},
	{method: http.MethodPost, path: "/credentials/:kind/:name/reveal"},
	{method: http.MethodPost, path: "/credentials/totp/:name/codes"},
	{method: http.MethodDelete, path: "/credentials/:kind/:name"},
	// VPN。
	{method: http.MethodGet, path: "/vpn", slow: true},
	{method: http.MethodPost, path: "/vpn/profiles"},
	{method: http.MethodPut, path: "/vpn/profiles/:name"},
	{method: http.MethodDelete, path: "/vpn/profiles/:name"},
	{method: http.MethodPost, path: "/vpn/profiles/:name/rename"},
	{method: http.MethodPost, path: "/vpn/profiles/:name/reveal"},
	{method: http.MethodGet, path: "/vpn/profiles/:name/logs"},
	{method: http.MethodPost, path: "/vpn/profiles/:name/route", slow: true},
	{method: http.MethodDelete, path: "/vpn/profiles/:name/route", slow: true},
	{method: http.MethodPut, path: "/vpn/bindings"},
	// Diagnostics。
	{method: http.MethodPost, path: "/diagnostics/config"},
	{method: http.MethodPost, path: "/diagnostics/effective"},
	{method: http.MethodPost, path: "/diagnostics/reachability", slow: true},
	{method: http.MethodPost, path: "/diagnostics/authentication", slow: true},
	// SFTP。転送の仕事は実行先のエンジンが持ち、「ローカル」は実行先のマシンのファイルである。
	// 接続先へつなぐのを待つ操作は slow、ファイルの中身を運ぶ操作は file にする。
	{method: http.MethodGet, path: "/sftp/transfers"},
	{method: http.MethodPost, path: "/sftp/transfers", slow: true},
	{method: http.MethodDelete, path: "/sftp/transfers/finished"},
	{method: http.MethodDelete, path: "/sftp/transfers/:id"},
	{method: http.MethodPut, path: "/sftp/transfers/settings"},
	{method: http.MethodPost, path: "/sftp/transfers/:id/queue-position"},
	{method: http.MethodPost, path: "/sftp/transfers/:id/actions"},
	{method: http.MethodPost, path: "/sftp/transfers/:id/download-checkpoint"},
	{method: http.MethodGet, path: "/sftp/compare", slow: true},
	{method: http.MethodGet, path: "/sftp/local/entries"},
	{method: http.MethodGet, path: "/sftp/local/text", file: true},
	{method: http.MethodPut, path: "/sftp/local/text", file: true},
	{method: http.MethodGet, path: "/sftp/local/preview", file: true},
	{method: http.MethodGet, path: "/sftp/local/thumbnail", file: true},
	{method: http.MethodGet, path: "/sftp/local/download", file: true},
	{method: http.MethodPost, path: "/sftp/media-tickets", slow: true},
	{method: http.MethodDelete, path: "/sftp/media-tickets/:ticket"},
	{method: http.MethodGet, path: "/sftp/media/:ticket", file: true},
	{method: http.MethodPost, path: "/sftp/local/directories"},
	{method: http.MethodPost, path: "/sftp/local/rename"},
	{method: http.MethodPost, path: "/sftp/local/delete-plan"},
	{method: http.MethodPost, path: "/sftp/local/delete"},
	{method: http.MethodGet, path: "/sftp/:alias/entries", slow: true},
	{method: http.MethodGet, path: "/sftp/:alias/connection-attempts/:attempt", slow: true},
	{method: http.MethodPost, path: "/sftp/:alias/connection-attempts/:attempt/host-key", slow: true},
	{method: http.MethodPost, path: "/sftp/:alias/entries", slow: true},
	{method: http.MethodGet, path: "/sftp/:alias/stats", slow: true},
	{method: http.MethodGet, path: "/sftp/:alias/preview", file: true},
	{method: http.MethodGet, path: "/sftp/:alias/thumbnail", file: true},
	{method: http.MethodGet, path: "/sftp/:alias/search", slow: true},
	{method: http.MethodGet, path: "/sftp/:alias/text", file: true},
	{method: http.MethodPut, path: "/sftp/:alias/text", file: true},
	{method: http.MethodGet, path: "/sftp/:alias/download", file: true},
	{method: http.MethodGet, path: "/sftp/:alias/archive", file: true},
	{method: http.MethodPost, path: "/sftp/:alias/uploads/:id", slow: true},
	{method: http.MethodPatch, path: "/sftp/:alias/uploads/:id", file: true},
	{method: http.MethodPost, path: "/sftp/:alias/uploads/:id/complete", slow: true},
	{method: http.MethodDelete, path: "/sftp/:alias/uploads/:id", slow: true},
	{method: http.MethodPatch, path: "/sftp/:alias/entry", slow: true},
	{method: http.MethodPatch, path: "/sftp/:alias/mode", slow: true},
	{method: http.MethodPost, path: "/sftp/:alias/mode-plan", slow: true},
	{method: http.MethodPatch, path: "/sftp/:alias/modes", slow: true},
	{method: http.MethodPost, path: "/sftp/:alias/symlink", slow: true},
	{method: http.MethodPatch, path: "/sftp/:alias/symlink", slow: true},
	{method: http.MethodPatch, path: "/sftp/:alias/ownership", slow: true},
	{method: http.MethodGet, path: "/sftp/:alias/space", slow: true},
	// Known Hosts。
	{method: http.MethodGet, path: "/known-hosts"},
	{method: http.MethodPost, path: "/known-hosts/delete"},
	{method: http.MethodPost, path: "/known-hosts/scan", slow: true},
	{method: http.MethodPost, path: "/known-hosts/add"},
}

// registerForwardedRoutes は、forwardedRoutes を /api/v1/engines/:alias の下に登録する。
func registerForwardedRoutes(engine *echo.Echo, handlers RemoteEngineHandlers) {
	for _, route := range forwardedRoutes {
		forward := handlers.Engines.ForwardAPI
		switch {
		case route.file:
			forward = handlers.Engines.ForwardFileAPI
		case route.slow:
			forward = handlers.Engines.ForwardSlowAPI
		}
		engine.Add(route.method, enginesPrefix+"/:alias"+route.path, handlers.forwarding(forward))
	}
}

// forwarding は、実行先のエンジンの、ターミナル以外の操作を forward で取り次ぐ受け口を返す。このマシンの
// セッションと CSRF は、ここへ届く前にこのマシンの規則で確かめてある。
func (h RemoteEngineHandlers) forwarding(forward func(http.ResponseWriter, *http.Request, string, string) error) echo.HandlerFunc {
	return func(c *echo.Context) error {
		alias, ok := aliasParam(c)
		if !ok {
			return problem(c, http.StatusBadRequest, "unsafe_alias")
		}
		remotePath := "/api/v1" + strings.TrimPrefix(c.Request().URL.Path, enginesPrefix+"/"+alias)
		if err := forward(c.Response(), c.Request(), alias, remotePath); err != nil {
			return remoteEngineProblem(c, err)
		}
		return nil
	}
}
