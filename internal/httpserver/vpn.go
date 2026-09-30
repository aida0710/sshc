package httpserver

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"

	"sshc/internal/application"
	"sshc/internal/session"
	"sshc/internal/vpn"
	"sshc/internal/vpnprofile"
)

// VPN 経路の設定と、いまの状態を扱う。
//
// 秘密は、一覧と保存の応答には現れない。保存のときに受け取って Vault へ渡し、取り出すのは
// 確認のトークンを添えた RevealSecrets だけにする。

// VPNHandlers は、プロファイルの保存と、経路の開始・停止を提供する。
//
// プロファイルの手順（設定と秘密をまとめて書く、経路を止めてから改名する）は
// vpnprofile.Service が持つ。ここは入力の検査と応答への変換だけをする。
type VPNHandlers struct {
	// Config は、一覧と接続の紐付けを読み書きする。
	Config   *application.Service
	Profiles *vpnprofile.Service
	VPN      *vpn.Manager
	// Actions は、秘密を取り出す確認のトークンを消費する。
	Actions ActionHandlers
}

func registerVPNRoutes(engine *echo.Echo, handlers VPNHandlers) {
	engine.GET("/api/v1/vpn", handlers.Overview)
	engine.POST("/api/v1/vpn/profiles", handlers.CreateProfile)
	engine.PUT("/api/v1/vpn/profiles/:name", handlers.UpdateProfile)
	engine.DELETE("/api/v1/vpn/profiles/:name", handlers.DeleteProfile)
	engine.POST("/api/v1/vpn/profiles/:name/rename", handlers.RenameProfile)
	engine.POST("/api/v1/vpn/profiles/:name/reveal", handlers.RevealSecrets)
	engine.GET("/api/v1/vpn/profiles/:name/logs", handlers.Logs)
	engine.POST("/api/v1/vpn/profiles/:name/session", handlers.StartSession)
	engine.DELETE("/api/v1/vpn/profiles/:name/session", handlers.StopSession)
	engine.PUT("/api/v1/vpn/bindings", handlers.SetBinding)
}

// Overview は、保存済みのプロファイルと、それぞれのいまの状態を返す。
//
// waitForRoutes=false なら、経路の状態を docker から読むのを待たない。まだ確かめて
// いなければ checking を付けて返す。画面は、プロファイルの一覧を先に見せ、経路の
// 状態はあとから読み直して埋める。
func (h VPNHandlers) Overview(c *echo.Context) error {
	wait := true
	if raw := c.QueryParam("waitForRoutes"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return problem(c, http.StatusBadRequest, "invalid_request")
		}
		wait = parsed
	}
	return h.overview(c, wait)
}

// CreateProfile は、新しいプロファイルを秘密と一緒に作る。同じ名前があれば断る。
//
// 秘密は応答に現れない。
func (h VPNHandlers) CreateProfile(c *echo.Context) error {
	var request VPNProfileRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	var secrets vpn.SecretsDocument
	if request.Secrets != nil {
		secrets = *request.Secrets
	}
	if err := h.Profiles.Create(request.Profile, secrets); err != nil {
		return vpnProblem(c, err)
	}
	return h.respond(c)
}

// UpdateProfile は、保存済みのプロファイルを置き換える。名前が無ければ断る。
//
// 秘密は省略でき、送られた項目だけが保存済みのものに重なる。設定だけを直すときに、
// 秘密を入れ直させない。
func (h VPNHandlers) UpdateProfile(c *echo.Context) error {
	var request VPNProfileRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if request.Profile.Name != c.Param("name") {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if err := h.Profiles.Update(request.Profile, request.Secrets); err != nil {
		return vpnProblem(c, err)
	}
	return h.respond(c)
}

// DeleteProfile は、プロファイルと、それを指している紐付けと、秘密を消す。
// 動いている経路は止める。
func (h VPNHandlers) DeleteProfile(c *echo.Context) error {
	if err := h.Profiles.Remove(c.Request().Context(), c.Param("name")); err != nil {
		return vpnProblem(c, err)
	}
	return h.respond(c)
}

// RenameProfile は、プロファイルの名前を変える。設定・秘密・接続の紐付けが
// 一緒に移る。
func (h VPNHandlers) RenameProfile(c *echo.Context) error {
	var request VPNRenameRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if err := h.Profiles.Rename(c.Request().Context(), c.Param("name"), request.Name); err != nil {
		return vpnProblem(c, err)
	}
	return h.respond(c)
}

// RevealSecrets は、保存済みの秘密を返す。画面は、編集を開いたときにこれをフォームへ入れる。
//
// 確認のトークン（kind は vpn_profile.reveal、target はプロファイルの名前）が要る。Vault が
// ロック中なら、ほかの取り出しと同じく vault_locked で断る。
func (h VPNHandlers) RevealSecrets(c *echo.Context) error {
	name := c.Param("name")
	if allowed, response := h.Actions.consume(c, session.ActionRevealVPNSecrets, name); !allowed {
		return response
	}
	secrets, err := h.Profiles.RevealSecrets(name)
	if err != nil {
		return vpnProblem(c, err)
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, secrets)
}

// addVPNActions は、VPN の秘密を取り出す確認を、いま保存されている秘密に結び付ける。
func addVPNActions(registry actionRegistry, profiles *vpnprofile.Service) {
	registry[session.ActionRevealVPNSecrets] = actionKind{
		evidence: func(_ context.Context, name string) (string, error) { return profiles.SecretsEvidence(name) },
		fail:     vpnProblem,
	}
}

// Logs は、engine がその経路を用意した記録とコンテナの直近の出力を、秘密を伏せて返す。
//
// 繋がらないときに最初に見る場所である。利用者に docker を直接叩かせない。
func (h VPNHandlers) Logs(c *echo.Context) error {
	name := c.Param("name")
	secrets, err := h.Profiles.LogRedactions(name)
	if err != nil {
		return vpnProblem(c, err)
	}
	lines, err := h.VPN.Logs(c.Request().Context(), name, secrets)
	if err != nil {
		return vpnProblem(c, err)
	}
	return c.JSON(http.StatusOK, VPNLogs{Lines: lines})
}

// StartSession は、プロファイルの経路を用意する。
func (h VPNHandlers) StartSession(c *echo.Context) error {
	profile, secrets, err := h.Profiles.Route(c.Param("name"))
	if err != nil {
		return vpnProblem(c, err)
	}
	if err := h.VPN.Start(c.Request().Context(), profile, secrets); err != nil {
		return vpnProblem(c, err)
	}
	return h.respond(c)
}

// StopSession は、利用者の求めで経路を切断する。切断で切れた接続は、自動再接続では
// 経路を起動し直さない。
func (h VPNHandlers) StopSession(c *echo.Context) error {
	if err := h.VPN.Disconnect(c.Request().Context(), c.Param("name")); err != nil {
		return vpnProblem(c, err)
	}
	return h.respond(c)
}

// SetBinding は、接続が通るプロファイルを決める。空なら紐付けを外す。
func (h VPNHandlers) SetBinding(c *echo.Context) error {
	var request VPNBindingRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if _, err := h.Config.SetConnectionVPN(request.Alias, request.Profile); err != nil {
		return vpnProblem(c, err)
	}
	return h.respond(c)
}

// respond は、いまの一覧と状態を返す。すべての操作がこれを返すので、呼び出し側は
// 変更のあとに状態を取り直さなくてよい。
func (h VPNHandlers) respond(c *echo.Context) error {
	return h.overview(c, true)
}

// overview は、いまの一覧と状態を返す。waitForRoutes が false なら、docker から
// 経路の状態を読むのを待たず、まだ確かめていなければ checking を付ける。
func (h VPNHandlers) overview(c *echo.Context, waitForRoutes bool) error {
	profiles, err := h.Config.VPNProfiles()
	if err != nil {
		return vpnProblem(c, err)
	}
	bindings, err := h.Config.VPNBindings()
	if err != nil {
		return vpnProblem(c, err)
	}
	response := VPNOverview{Available: true, Profiles: make([]VPNProfileStatus, 0, len(profiles))}
	var statuses map[string]vpn.Status
	if waitForRoutes {
		// docker が見つからない、daemon が応えない、のどちらもここで分かる。
		statuses, err = h.VPN.Statuses(c.Request().Context())
	} else {
		var known bool
		statuses, known, err = h.VPN.KnownStatuses()
		response.Checking = !known
	}
	if err != nil {
		response.Available, response.Unavailable, response.Detail = false, unavailableReason(err), err.Error()
	}
	for _, profile := range profiles {
		entry := VPNProfileStatus{Profile: profile, Connections: bindings[profile.Name]}
		if entry.Connections == nil {
			entry.Connections = []string{}
		}
		if status, present := statuses[profile.Name]; present {
			entry.Running, entry.RelaySocket, entry.Phase = status.Running, status.RelaySocket, status.Phase
			entry.OpenConnections = status.OpenConnections
			if status.Tunnel != (vpn.TunnelStatus{}) {
				entry.Tunnel = &VPNTunnel{
					Interface: status.Tunnel.Interface, Address: status.Tunnel.Address,
					Since: status.Tunnel.Since, Backend: status.Tunnel.Backend,
				}
			}
		}
		response.Profiles = append(response.Profiles, entry)
	}
	return c.JSON(http.StatusOK, response)
}

// unavailableReason は、VPN 経路を使えない理由の語を返す。画面と CLI はこれを訳して
// 見せる。Detail の生の文は、訳せない理由を調べるためだけに添える。
func unavailableReason(err error) VPNUnavailable {
	if errors.Is(err, vpn.ErrDockerMissing) {
		return VPNDockerMissing
	}
	// docker が動いていない。見つかったあとで一覧の読み取りに失敗した場合も、
	// daemon が応えなくなったのである。
	return VPNDockerNotRunning
}
