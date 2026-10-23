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
	var connectErr *remoteengine.ConnectError
	var incompatible *remoteengine.IncompatibleEngineError
	switch {
	case errors.Is(err, remoteengine.ErrClosed):
		return problem(c, http.StatusServiceUnavailable, "server_stopping")
	case errors.Is(err, remoteengine.ErrNotAdded):
		return problem(c, http.StatusNotFound, "remote_engine_not_added")
	case errors.Is(err, remoteengine.ErrRequestTooLarge):
		return problem(c, http.StatusRequestEntityTooLarge, "request_body_too_large")
	case errors.As(err, &connectErr):
		return problemWith(c, http.StatusBadGateway, problemPayload{
			Code: "remote_engine_connect_failed", Reason: remoteEngineConnectFailure(connectErr.Err),
		})
	case errors.As(err, &incompatible):
		// detail は接続先のバージョンだけである。このマシンのバージョンは画面と CLI が知っている。
		return problemDetail(c, http.StatusConflict, "remote_engine_incompatible", incompatible.Remote)
	case errors.Is(err, remoteengine.ErrNotRunning):
		return problem(c, http.StatusBadGateway, "remote_engine_not_running")
	case errors.Is(err, remoteengine.ErrUnproven):
		return problem(c, http.StatusBadGateway, "remote_engine_unproven")
	case errors.Is(err, remoteengine.ErrNotInstalled):
		return problem(c, http.StatusBadGateway, "remote_engine_not_installed")
	case errors.Is(err, remoteengine.ErrForwardingRefused):
		return problem(c, http.StatusBadGateway, "remote_engine_forwarding_refused")
	case errors.Is(err, remoteengine.ErrNoEntrance):
		return problem(c, http.StatusBadGateway, "remote_engine_no_entrance")
	case errors.Is(err, remoteengine.ErrWrongPassphrase):
		// 断ったのは接続先の Vault で、このマシンのセッションではない。403 でも、画面は
		// invalid_csrf 以外をサインインのやり直しと読まない。
		return problem(c, http.StatusForbidden, "remote_engine_wrong_passphrase")
	case errors.Is(err, remoteengine.ErrUnlockRefused):
		return problem(c, http.StatusBadGateway, "remote_engine_unlock_failed")
	}
	return unexpectedProblem(c, "remote_engine_failed", err)
}
