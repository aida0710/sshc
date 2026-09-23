package httpserver

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/application"
	"sshc/internal/secret"
	"sshc/internal/vpn"
)

// VPN 経路の設定と、いまの状態を扱う。
//
// 秘密は応答に現れない。保存のときだけ受け取り、Vault へ渡す。

// VPNHandlers は、プロファイルの保存と、経路の開始・停止を提供する。
type VPNHandlers struct {
	Config   *application.Service
	Secrets  *secret.Service
	Sessions *vpn.Manager
}

type vpnOverviewResponse struct {
	Available bool                 `json:"available"`
	Detail    string               `json:"detail,omitempty"`
	Profiles  []vpnSessionResponse `json:"profiles"`
}

type vpnSessionResponse struct {
	Profile application.VPNProfile `json:"profile"`
	Running bool                   `json:"running"`
	// RelaySocket は、中継のソケットの場所である。開いていなければ空になる。
	RelaySocket string   `json:"relaySocket"`
	Connections []string `json:"connections"`
	// Tunnel は、コンテナの中のトンネルの様子である。経路が無ければ省略する。
	Tunnel *vpnTunnelResponse `json:"tunnel,omitempty"`
	// Phase は、いま経路を用意している段階である。用意していなければ空。
	Phase vpn.StartPhase `json:"phase,omitempty"`
}

type vpnTunnelResponse struct {
	Interface string `json:"interface,omitempty"`
	Address   string `json:"address,omitempty"`
	Since     string `json:"since,omitempty"`
	Backend   string `json:"backend,omitempty"`
}

type vpnLogsResponse struct {
	Lines string `json:"lines"`
}

type vpnProfileRequest struct {
	Profile application.VPNProfile `json:"profile"`
	Secrets *vpnSecretsRequest     `json:"secrets,omitempty"`
}

type vpnSecretsRequest struct {
	WireGuardPrivateKey string `json:"wireguardPrivateKey,omitempty"`
	L2TPPassword        string `json:"l2tpPassword,omitempty"`
	IPsecPSK            string `json:"ipsecPsk,omitempty"`
}

type vpnRenameRequest struct {
	Name string `json:"name"`
}

type vpnBindingRequest struct {
	Alias   string `json:"alias"`
	Profile string `json:"profile"`
}

func registerVPNRoutes(engine *echo.Echo, handlers VPNHandlers) {
	engine.GET("/api/v1/vpn", handlers.Overview)
	engine.PUT("/api/v1/vpn/profiles/:name", handlers.SaveProfile)
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

// SaveProfile は、プロファイルひとつを保存する。
//
// 秘密は同じ要求で受け取るが、応答には現れない。省略された場合は保存済みの
// 秘密を残す。設定だけを直すときに、秘密を入れ直させない。
func (h VPNHandlers) SaveProfile(c *echo.Context) error {
	var request vpnProfileRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if request.Profile.Name != c.Param("name") {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if _, err := h.Config.SaveVPNProfile(request.Profile); err != nil {
		return vpnProblem(c, err)
	}
	if request.Secrets != nil {
		document, err := vpn.EncodeSecrets(vpn.Secrets{
			WireGuardPrivateKey: request.Secrets.WireGuardPrivateKey,
			L2TPPassword:        request.Secrets.L2TPPassword,
			IPsecPSK:            request.Secrets.IPsecPSK,
		})
		if err != nil {
			return vpnProblem(c, err)
		}
		if err := h.Secrets.SetVPNSecrets(request.Profile.Name, document); err != nil {
			return vpnProblem(c, err)
		}
	}
	return h.respond(c)
}

// DeleteProfile は、プロファイルと、それを指している紐付けと、秘密を消す。
//
// metadata を先に書く。秘密だけが残っても、それを指すものがもう無い。逆順だと、
// 秘密の無いプロファイルを指したままの接続が残りうる。
func (h VPNHandlers) DeleteProfile(c *echo.Context) error {
	name := c.Param("name")
	if _, err := h.Config.RemoveVPNProfile(name); err != nil {
		return vpnProblem(c, err)
	}
	if err := h.Secrets.RemoveVPNSecrets(name); err != nil && !errors.Is(err, secret.ErrLocked) {
		return vpnProblem(c, err)
	}
	// 経路が動いていれば止める。設定が無くなったあとも残っているコンテナは、
	// 誰も説明できない状態である。
	if err := h.Sessions.Stop(c.Request().Context(), name); err != nil && !errors.Is(err, vpn.ErrDockerMissing) {
		return vpnProblem(c, err)
	}
	return h.respond(c)
}

// RenameProfile は、プロファイルの名前を変える。
//
// 設定・秘密・接続の紐付けは、名前でつながっている。どれか一つだけを直すと、
// 残りが古い名前を指したままになる。動いている経路はコンテナの名前も変わるので、
// 先に畳む。
func (h VPNHandlers) RenameProfile(c *echo.Context) error {
	var request vpnRenameRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	from := c.Param("name")
	if err := h.Sessions.Stop(c.Request().Context(), from); err != nil &&
		!errors.Is(err, vpn.ErrDockerMissing) && !errors.Is(err, vpn.ErrSessionForeign) {
		return vpnProblem(c, err)
	}
	if _, err := h.Config.RenameVPNProfile(from, request.Name); err != nil {
		return vpnProblem(c, err)
	}
	if err := h.Secrets.RenameVPNSecrets(from, request.Name); err != nil &&
		!errors.Is(err, secret.ErrLocked) && !errors.Is(err, secret.ErrUnknownCredential) {
		return vpnProblem(c, err)
	}
	return h.respond(c)
}

// Logs は、そのコンテナの直近の出力を、秘密を伏せて返す。
//
// 繋がらないときに最初に見る場所である。利用者に docker を直接叩かせない。
func (h VPNHandlers) Logs(c *echo.Context) error {
	name := c.Param("name")
	if _, err := h.Config.VPNProfile(name); err != nil {
		return vpnProblem(c, err)
	}
	// 秘密は伏せるためだけに読む。読めなくてもログは返す。伏せる相手が
	// 分からないときは、伏せられる範囲で伏せる。
	var secrets vpn.Secrets
	if stored, err := h.Secrets.VPNSecrets(name); err == nil {
		secrets, _ = vpn.DecodeSecrets(stored)
	}
	lines, err := h.Sessions.Logs(c.Request().Context(), name, secrets)
	if err != nil {
		return vpnProblem(c, err)
	}
	return c.JSON(http.StatusOK, vpnLogsResponse{Lines: lines})
}

// StartSession は、プロファイルの経路を用意する。
func (h VPNHandlers) StartSession(c *echo.Context) error {
	profile, secrets, err := h.route(c.Param("name"))
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
	var request vpnBindingRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if _, err := h.Config.SetConnectionVPN(request.Alias, request.Profile); err != nil {
		return vpnProblem(c, err)
	}
	return h.respond(c)
}

// route は、保存済みの設定と秘密をひとつの経路として集める。
func (h VPNHandlers) route(name string) (vpn.Profile, vpn.Secrets, error) {
	profile, err := h.Config.VPNProfile(name)
	if err != nil {
		return vpn.Profile{}, vpn.Secrets{}, err
	}
	stored, err := h.Secrets.VPNSecrets(name)
	if err != nil {
		return vpn.Profile{}, vpn.Secrets{}, err
	}
	secrets, err := vpn.DecodeSecrets(stored)
	if err != nil {
		return vpn.Profile{}, vpn.Secrets{}, err
	}
	return profile, secrets, nil
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
	response := vpnOverviewResponse{Available: true, Profiles: make([]vpnSessionResponse, 0, len(profiles))}
	if err := h.Sessions.Available(c.Request().Context()); err != nil {
		response.Available = false
		response.Detail = err.Error()
	}
	for _, profile := range profiles {
		session := vpnSessionResponse{Profile: profile, Connections: bindings[profile.Name]}
		if session.Connections == nil {
			session.Connections = []string{}
		}
		if response.Available {
			status, err := h.Sessions.Status(c.Request().Context(), profile.Name)
			if err == nil {
				session.Running, session.RelaySocket = status.Running, status.RelaySocket
				session.Phase = status.Phase
				if status.Tunnel != (vpn.TunnelStatus{}) {
					session.Tunnel = &vpnTunnelResponse{
						Interface: status.Tunnel.Interface, Address: status.Tunnel.Address,
						Since: status.Tunnel.Since, Backend: status.Tunnel.Backend,
					}
				}
			}
		}
		response.Profiles = append(response.Profiles, session)
	}
	return c.JSON(http.StatusOK, response)
}

// vpnProblem は、VPN の拒否を画面とCLIが扱える応答へ直す。
func vpnProblem(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, application.ErrUnknownVPNProfile):
		return problem(c, http.StatusNotFound, "vpn_profile_unknown")
	case errors.Is(err, application.ErrUnknownConnection):
		return problem(c, http.StatusNotFound, "connection_unknown")
	case errors.Is(err, application.ErrMetadataVPN), errors.Is(err, vpn.ErrProfileName),
		errors.Is(err, vpn.ErrBackend), errors.Is(err, vpn.ErrTarget), errors.Is(err, vpn.ErrSettings):
		return problem(c, http.StatusBadRequest, "vpn_profile_invalid")
	case errors.Is(err, vpn.ErrSecrets), errors.Is(err, secret.ErrUnknownCredential):
		return problem(c, http.StatusConflict, "vpn_secrets_missing")
	case errors.Is(err, secret.ErrLocked):
		return problem(c, http.StatusConflict, "vault_locked")
	case errors.Is(err, vpn.ErrDockerMissing):
		return problem(c, http.StatusConflict, "vpn_docker_missing")
	case errors.Is(err, vpn.ErrTunnelDevice):
		return problem(c, http.StatusConflict, "vpn_tunnel_device_missing")
	case errors.Is(err, vpn.ErrImageBuild):
		return problem(c, http.StatusConflict, "vpn_image_build_failed")
	case errors.Is(err, vpn.ErrSessionForeign):
		return problem(c, http.StatusConflict, "vpn_container_foreign")
	case errors.Is(err, vpn.ErrSessionFailed):
		return problem(c, http.StatusConflict, "vpn_session_failed")
	}
	return serviceProblem(c, err)
}
