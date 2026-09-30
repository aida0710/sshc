package httpserver

import (
	"errors"
	"net"
	"net/http"
	"net/netip"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/browserauth"
	"sshc/internal/session"
)

type Handlers struct {
	Sessions    *session.Manager
	BrowserAuth *browserauth.Store
	Version     string
	// PeerMayBelongToAnotherUser は、TCP 接続が同じマシンの別 OS ユーザーのものかも
	// しれないかを返す。持ち主を読む前に client が閉じた接続も、そうかもしれないものに
	// 含める。nil なら確かめない。
	PeerMayBelongToAnotherUser func(client, server netip.AddrPort) bool
}

// Renew は、cookie と現在の CSRF token が指しているセッションに対して新しい
// CSRF token を発行する。
//
// middleware が先に現在の token を検証する。cookie は port に束縛されないので、
// localhost の別 server が受け取った cookie だけから token を再発行してはならない。
// reload に必要な token は browser の port-origin scoped sessionStorage に残す。
func (h Handlers) Renew(c *echo.Context) error {
	if h.Sessions == nil {
		return unexpectedProblem(c, "bootstrap_failed", nil)
	}
	cookie, err := c.Request().Cookie(SessionCookie)
	if err != nil {
		return problem(c, http.StatusUnauthorized, "session_required")
	}
	csrf, ok := h.Sessions.RenewCSRF(cookie.Value, c.Request().Header.Get(CSRFHeader))
	if !ok {
		return problem(c, http.StatusUnauthorized, "invalid_session")
	}
	return c.JSON(http.StatusOK, api.BootstrapResponse{CsrfToken: csrf})
}

func (h Handlers) Bootstrap(c *echo.Context) error {
	if h.Sessions == nil {
		return unexpectedProblem(c, "bootstrap_failed", nil)
	}
	// bootstrap を消費する前に断る。正規のブラウザがあとから同じ bootstrap で入れる。
	if h.mayBeFromAnotherOSUser(c.Request()) {
		return problem(c, http.StatusForbidden, "bootstrap_forbidden")
	}

	existing := sessionCookie(c.Request())
	credentials, setCookie, err := h.Sessions.BootstrapForSession(
		c.Request().Header.Get("X-SSHC-Bootstrap"), existing,
	)
	switch {
	case errors.Is(err, session.ErrInvalidBootstrap):
		return problem(c, http.StatusUnauthorized, "invalid_bootstrap")
	case errors.Is(err, session.ErrBootstrapUsed):
		return problem(c, http.StatusConflict, "bootstrap_used")
	case err != nil:
		return unexpectedProblem(c, "bootstrap_failed", err)
	}

	response := api.BootstrapResponse{CsrfToken: credentials.CSRFToken}
	if h.BrowserAuth != nil {
		browserToken, issued, registerErr := h.BrowserAuth.Register(c.Request().Header.Get("X-SSHC-Browser"))
		if registerErr != nil {
			if setCookie {
				h.Sessions.Revoke(credentials.SessionID)
			}
			return unexpectedProblem(c, "browser_registration_failed", registerErr)
		}
		if issued {
			response.BrowserToken = &browserToken
		}
	}
	if setCookie {
		setSessionCookie(c, credentials.SessionID)
	}
	return c.JSON(http.StatusOK, response)
}

// mayBeFromAnotherOSUser は、要求の TCP 接続を同じマシンの別 OS ユーザーが張った
// かもしれないかを返す。
//
// Linux では、ブラウザへ渡した bootstrap URL を別 OS ユーザーも
// /proc/<pid>/cmdline から読める。bootstrap は最初に提示した者が使えるので、
// 読まれても使わせないよう、接続の持ち主で断る。要求を送ってすぐ閉じた接続は
// 持ち主を読めないので、これも断る。正規のブラウザは応答を待つので、閉じた接続を
// 断っても困らない。
func (h Handlers) mayBeFromAnotherOSUser(request *http.Request) bool {
	if h.PeerMayBelongToAnotherUser == nil {
		return false
	}
	client, err := netip.ParseAddrPort(request.RemoteAddr)
	if err != nil {
		return false
	}
	local, ok := request.Context().Value(http.LocalAddrContextKey).(net.Addr)
	if !ok {
		return false
	}
	server, err := netip.ParseAddrPort(local.String())
	if err != nil {
		return false
	}
	return h.PeerMayBelongToAnotherUser(client, server)
}

// Recover restores a browser session from the device-local enrolment capability. It is
// intentionally separate from cookie authentication: engine restart invalidates every
// in-memory cookie, while the browser registration remains valid on this fixed origin.
// Every recovery rotates the registration token; the response carries the replacement
// and the browser must store it before it recovers again.
func (h Handlers) Recover(c *echo.Context) error {
	if h.Sessions == nil || h.BrowserAuth == nil {
		return problem(c, http.StatusUnauthorized, "browser_registration_required")
	}
	entry, err := h.entrance().Recover(sessionCookie(c.Request()), c.Request().Header.Get("X-SSHC-Browser"))
	switch {
	case errors.Is(err, browserauth.ErrRegistrationRejected):
		return problem(c, http.StatusUnauthorized, "invalid_browser_registration")
	case errors.Is(err, browserauth.ErrSessionNotIssued):
		return unexpectedProblem(c, "bootstrap_failed", err)
	case err != nil:
		return unexpectedProblem(c, "browser_registration_failed", err)
	}
	if entry.SetCookie {
		setSessionCookie(c, entry.Credentials.SessionID)
	}
	return c.JSON(http.StatusOK, api.BootstrapResponse{CsrfToken: entry.Credentials.CSRFToken, BrowserToken: &entry.BrowserToken})
}

// SignOut は、このブラウザーのセッションを消し、登録 token が添えられていれば
// 登録と、その登録から入ったほかのセッションも消す。cookie と CSRF の検査は他の
// API と同じ経路で済んでいる。登録を消すと engine を再起動しても入り直せないので、
// 次に入るには `sshc open` の URL が要る。
func (h Handlers) SignOut(c *echo.Context) error {
	if h.Sessions == nil {
		return unexpectedProblem(c, "bootstrap_failed", nil)
	}
	if err := h.entrance().SignOut(sessionCookie(c.Request()), c.Request().Header.Get("X-SSHC-Browser")); err != nil {
		return unexpectedProblem(c, "browser_registration_failed", err)
	}
	clearSessionCookie(c)
	return c.NoContent(http.StatusNoContent)
}

func (h Handlers) entrance() browserauth.Entrance {
	return browserauth.Entrance{Registrations: h.BrowserAuth, Sessions: h.Sessions}
}

func sessionCookie(request *http.Request) string {
	cookie, err := request.Cookie(SessionCookie)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func setSessionCookie(c *echo.Context, sessionID string) {
	c.SetCookie(&http.Cookie{
		Name:     SessionCookie,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

func clearSessionCookie(c *echo.Context) {
	c.SetCookie(&http.Cookie{
		Name:     SessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

func (h Handlers) Health(c *echo.Context) error {
	return c.JSON(http.StatusOK, api.HealthResponse{Status: "ok", Version: h.Version})
}
