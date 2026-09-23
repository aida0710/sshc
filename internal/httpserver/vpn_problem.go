package httpserver

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/application"
	"sshc/internal/secret"
	"sshc/internal/vpn"
)

// vpnRefusal は、VPN の拒否ひとつと、その応答の状態とコードである。
type vpnRefusal struct {
	kind   error
	status int
	code   string
}

// vpnRefusals は、VPN の操作が断る理由と、画面と CLI が見分けるコードの対応である。
// 上から順に照らし、最初に当たったものを返す。
var vpnRefusals = []vpnRefusal{
	{application.ErrUnknownVPNProfile, http.StatusNotFound, "vpn_profile_unknown"},
	{application.ErrVPNProfileExists, http.StatusConflict, "vpn_profile_exists"},
	{application.ErrUnknownConnection, http.StatusNotFound, "connection_unknown"},
	{vpn.ErrTargetMismatch, http.StatusBadRequest, "vpn_target_mismatch"},
	{application.ErrMetadataVPN, http.StatusBadRequest, "vpn_profile_invalid"},
	{vpn.ErrProfileName, http.StatusBadRequest, "vpn_profile_invalid"},
	{vpn.ErrBackend, http.StatusBadRequest, "vpn_profile_invalid"},
	{vpn.ErrTarget, http.StatusBadRequest, "vpn_profile_invalid"},
	{vpn.ErrSettings, http.StatusBadRequest, "vpn_profile_invalid"},
	{vpn.ErrSecrets, http.StatusConflict, "vpn_secrets_missing"},
	{secret.ErrUnknownCredential, http.StatusConflict, "vpn_secrets_missing"},
	{secret.ErrLocked, http.StatusConflict, "vault_locked"},
	{secret.ErrNoVault, http.StatusNotFound, "vault_missing"},
	{vpn.ErrDockerMissing, http.StatusConflict, "vpn_docker_missing"},
	{vpn.ErrTunnelDevice, http.StatusConflict, "vpn_tunnel_device_missing"},
	{vpn.ErrImageBuild, http.StatusConflict, "vpn_image_build_failed"},
	{vpn.ErrSessionForeign, http.StatusConflict, "vpn_container_foreign"},
	{vpn.ErrSessionFailed, http.StatusConflict, "vpn_session_failed"},
}

// vpnProblem は、VPN の拒否を画面と CLI が扱える応答へ直す。
//
// 項目の誤りには `field`・`reason`・`limit` を、経路を用意できなかったときには
// `reason` を添える。どれも Go が決めた語だけで、利用者の入力や秘密、ログの
// 断片は載せない。
func vpnProblem(c *echo.Context, err error) error {
	for _, refusal := range vpnRefusals {
		if !errors.Is(err, refusal.kind) {
			continue
		}
		payload := problemPayload{Code: refusal.code}
		var fieldError *vpn.FieldError
		if errors.As(err, &fieldError) {
			payload.Field, payload.Reason, payload.Limit = fieldError.Field, string(fieldError.Reason), fieldError.Limit
		}
		var failure *vpn.SessionFailure
		if errors.As(err, &failure) {
			payload.Reason = string(failure.Reason)
		}
		return problemWith(c, refusal.status, payload)
	}
	return serviceProblem(c, err)
}
