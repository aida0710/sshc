package httpserver

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/application"
	"sshc/internal/vpn"
	"sshc/internal/vpnprofile"
)

// VPN 経路の設定と、いまの状態を扱う。
//
// 秘密は応答に現れない。保存のときだけ受け取り、Vault へ渡す。

// VPNHandlers は、プロファイルの保存と、経路の開始・停止を提供する。
//
// プロファイルの手順（設定と秘密をまとめて書く、経路を止めてから改名する）は
// vpnprofile.Service が持つ。ここは入力の検査と応答への変換だけをする。
type VPNHandlers struct {
	// Config は、一覧と接続の紐付けを読み書きする。
	Config   *application.Service
	Profiles *vpnprofile.Service
	Sessions *vpn.Manager
}

func registerVPNRoutes(engine *echo.Echo, handlers VPNHandlers) {
	engine.GET("/api/v1/vpn", handlers.Overview)
	engine.POST("/api/v1/vpn/profiles", handlers.CreateProfile)
	engine.PUT("/api/v1/vpn/profiles/:name", handlers.UpdateProfile)
	engine.DELETE("/api/v1/vpn/profiles/:name", handlers.DeleteProfile)
	engine.POST("/api/v1/vpn/profiles/:name/rename", handlers.RenameProfile)
	engine.GET("/api/v1/vpn/profiles/:name/logs", handlers.Logs)
	engine.POST("/api/v1/vpn/profiles/:name/session", handlers.StartSession)
	engine.DELETE("/api/v1/vpn/profiles/:name/session", handlers.StopSession)
	engine.PUT("/api/v1/vpn/bindings", handlers.SetBinding)
}

// Overview は、保存済みのプロファイルと、それぞれのいまの状態を返す。
func (h VPNHandlers) Overview(c *echo.Context) error {
	return h.respond(c)
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

// Logs は、そのコンテナの直近の出力を、秘密を伏せて返す。
//
// 繋がらないときに最初に見る場所である。利用者に docker を直接叩かせない。
func (h VPNHandlers) Logs(c *echo.Context) error {
	name := c.Param("name")
	secrets, err := h.Profiles.LogRedactions(name)
	if err != nil {
		return vpnProblem(c, err)
	}
	lines, err := h.Sessions.Logs(c.Request().Context(), name, secrets)
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
	if err := h.Sessions.Start(c.Request().Context(), profile, secrets); err != nil {
		return vpnProblem(c, err)
	}
	return h.respond(c)
}

// StopSession は、経路を畳む。
func (h VPNHandlers) StopSession(c *echo.Context) error {
	if err := h.Sessions.Stop(c.Request().Context(), c.Param("name")); err != nil {
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
	profiles, err := h.Config.VPNProfiles()
	if err != nil {
		return vpnProblem(c, err)
	}
	bindings, err := h.Config.VPNBindings()
	if err != nil {
		return vpnProblem(c, err)
	}
	ctx := c.Request().Context()
	response := VPNOverview{Available: true, Profiles: make([]VPNProfileStatus, 0, len(profiles))}
	var statuses map[string]vpn.Status
	if err := h.Sessions.Available(ctx); err != nil {
		response.Available, response.Detail = false, err.Error()
	} else if statuses, err = h.Sessions.Statuses(ctx); err != nil {
		// docker は見つかっているが、daemon が応えなくなった。
		response.Available, response.Detail = false, err.Error()
	}
	for _, profile := range profiles {
		entry := VPNProfileStatus{Profile: profile, Connections: bindings[profile.Name]}
		if entry.Connections == nil {
			entry.Connections = []string{}
		}
		if status, present := statuses[profile.Name]; present {
			entry.Running, entry.RelaySocket, entry.Phase = status.Running, status.RelaySocket, status.Phase
			if status.Tunnel != (vpn.TunnelStatus{}) {
				entry.Tunnel = &VPNTunnel{
					Interface: status.Tunnel.Interface, Address: status.Tunnel.Address,
					Since: status.Tunnel.Since, Backend: status.Tunnel.Backend,
					TargetAddress: status.Tunnel.TargetAddress,
				}
			}
		}
		response.Profiles = append(response.Profiles, entry)
	}
	return c.JSON(http.StatusOK, response)
}
