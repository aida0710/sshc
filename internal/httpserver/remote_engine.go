package httpserver

import (
	"context"
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/remoteengine"
	"sshc/internal/sshclient"
	"sshc/internal/validate"
)

// RemoteEnginePath は、CLI（`sshc open <alias>`）が、SSH の接続先で動いているエンジンを
// このマシンのブラウザで開く URL を受け取るエンドポイントである。
const RemoteEnginePath = "/cli/remote-engine"

// RemoteEngineOpener は、接続先のエンジンへの中継を開き、ブラウザで開く URL を返す。
type RemoteEngineOpener interface {
	Open(ctx context.Context, alias string) (string, error)
}

type remoteEngineRequest struct {
	Alias string `json:"alias"`
}

// remoteEngineConnectFailure は、接続先への SSH の接続ができなかった理由の語である。
// CLI が理由ごとの直し方を添える。
func remoteEngineConnectFailure(err error) string {
	switch {
	case errors.Is(err, sshclient.ErrHostKeyUnknown):
		return "host_key_unknown"
	case errors.Is(err, sshclient.ErrPromptUnavailable):
		return "prompt_unavailable"
	}
	return "connection_failed"
}

// OpenRemoteEngine は、接続先のエンジンを開く URL を返す。返す URL は Open と同じく
// 一度だけ使える入口であり、ログへ残さない。
func (h CLIHandlers) OpenRemoteEngine(c *echo.Context) error {
	request := c.Request()
	if request.Header.Get(echo.HeaderContentType) != "application/json" {
		return problem(c, http.StatusUnsupportedMediaType, "unsupported_media_type")
	}
	var decoded remoteEngineRequest
	if err := decodeJSONWithin(c, maxConnectBody, &decoded); err != nil {
		if errors.Is(err, errBodyTooLarge) {
			return problem(c, http.StatusRequestEntityTooLarge, "request_body_too_large")
		}
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if err := validate.Alias(decoded.Alias); err != nil {
		return problem(c, http.StatusBadRequest, "unsafe_alias")
	}

	opened, err := h.RemoteEngines.Open(request.Context(), decoded.Alias)
	if err != nil {
		return remoteEngineProblem(c, err)
	}
	return c.JSON(http.StatusOK, openResponse{URL: opened})
}

// remoteEngineProblem は、接続先のエンジンを開けなかった理由を、理由ごとの code で断る。
// CLI と画面が、それぞれの言葉で直し方を添える。
func remoteEngineProblem(c *echo.Context, err error) error {
	status, payload, known := remoteEngineFailure(err)
	if !known {
		return unexpectedProblem(c, payload.Code, err)
	}
	return problemWith(c, status, payload)
}

// remoteEngineFailure は、実行先のエンジンについての失敗を、状態コードと problem の本文にする。
// known が false なら想定外の失敗で、本文は remote_engine_failed である。要求への応答と、
// 終わった操作の記録（ActionLog）の problem が、同じ失敗に同じ code を使うよう、ここで一度だけ決める。
func remoteEngineFailure(err error) (status int, payload problemPayload, known bool) {
	var connectErr *remoteengine.ConnectError
	var incompatible *remoteengine.IncompatibleEngineError
	var refusal *remoteengine.RemoteRefusalError
	switch {
	case errors.As(err, &refusal):
		// 接続先へ届かなかったか、接続先が資格や Vault を断った。語は取り次ぎの断りと同じである。
		return refusal.Status, problemPayload{Code: refusal.Code}, true
	case errors.Is(err, remoteengine.ErrClosed):
		return http.StatusServiceUnavailable, problemPayload{Code: "server_stopping"}, true
	case errors.Is(err, remoteengine.ErrNotAdded):
		return http.StatusNotFound, problemPayload{Code: "remote_engine_not_added"}, true
	case errors.Is(err, remoteengine.ErrRequestTooLarge):
		return http.StatusRequestEntityTooLarge, problemPayload{Code: "request_body_too_large"}, true
	case errors.As(err, &connectErr):
		return http.StatusBadGateway, problemPayload{
			Code: "remote_engine_connect_failed", Reason: remoteEngineConnectFailure(connectErr.Err),
		}, true
	case errors.As(err, &incompatible):
		// detail は接続先のバージョンだけである。このマシンのバージョンは画面と CLI が知っている。
		return http.StatusConflict, problemPayload{Code: "remote_engine_incompatible", Detail: boundedProblemDetail(incompatible.Remote)}, true
	case errors.Is(err, remoteengine.ErrNotRunning):
		return http.StatusBadGateway, problemPayload{Code: "remote_engine_not_running"}, true
	case errors.Is(err, remoteengine.ErrUnproven):
		return http.StatusBadGateway, problemPayload{Code: "remote_engine_unproven"}, true
	case errors.Is(err, remoteengine.ErrNotInstalled):
		return http.StatusBadGateway, problemPayload{Code: "remote_engine_not_installed"}, true
	case errors.Is(err, remoteengine.ErrForwardingRefused):
		return http.StatusBadGateway, problemPayload{Code: "remote_engine_forwarding_refused"}, true
	case errors.Is(err, remoteengine.ErrNoEntrance):
		return http.StatusBadGateway, problemPayload{Code: "remote_engine_no_entrance"}, true
	case errors.Is(err, remoteengine.ErrWrongPassphrase):
		// 断ったのは接続先の Vault で、このマシンのセッションではない。403 でも、画面は
		// invalid_csrf 以外をサインインのやり直しと読まない。
		return http.StatusForbidden, problemPayload{Code: "remote_engine_wrong_passphrase"}, true
	case errors.Is(err, remoteengine.ErrUnlockRefused):
		return http.StatusBadGateway, problemPayload{Code: "remote_engine_unlock_failed"}, true
	case errors.Is(err, remoteengine.ErrActionTimedOut):
		return http.StatusGatewayTimeout, problemPayload{Code: "remote_engine_action_timed_out"}, true
	}
	return http.StatusInternalServerError, problemPayload{Code: "remote_engine_failed"}, false
}
