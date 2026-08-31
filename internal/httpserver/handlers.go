package httpserver

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/browserauth"
	"sshc/internal/session"
)

type Handlers struct {
	Sessions    *session.Manager
	BrowserAuth *browserauth.Store
	Version     string
}

// Renew は、cookie と現在の CSRF token が指しているセッションに対して新しい
// CSRF token を発行する。
//
// middleware が先に現在の token を検証する。cookie は port に束縛されないので、
// localhost の別 server が受け取った cookie だけから token を再発行してはならない。
// reload に必要な token は browser の port-origin scoped sessionStorage に残す。
func (h Handlers) Renew(c *echo.Context) error {
	if h.Sessions == nil {
		return problem(c, http.StatusInternalServerError, "bootstrap_failed")
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
		return problem(c, http.StatusInternalServerError, "bootstrap_failed")
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
		return problem(c, http.StatusInternalServerError, "bootstrap_failed")
	}

	response := api.BootstrapResponse{CsrfToken: credentials.CSRFToken}
	if h.BrowserAuth != nil {
		browserToken, issued, registerErr := h.BrowserAuth.Register(c.Request().Header.Get("X-SSHC-Browser"))
		if registerErr != nil {
			if setCookie {
				h.Sessions.Revoke(credentials.SessionID)
			}
			return problem(c, http.StatusInternalServerError, "browser_registration_failed")
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

// Recover restores a browser session from the device-local enrolment capability. It is
// intentionally separate from cookie authentication: engine restart invalidates every
// in-memory cookie, while the browser registration remains valid on this fixed origin.
func (h Handlers) Recover(c *echo.Context) error {
	if h.Sessions == nil || h.BrowserAuth == nil {
		return problem(c, http.StatusUnauthorized, "browser_registration_required")
	}
	if !h.BrowserAuth.Verify(c.Request().Header.Get("X-SSHC-Browser")) {
		return problem(c, http.StatusUnauthorized, "invalid_browser_registration")
	}
	credentials, setCookie, err := h.Sessions.JoinOrIssue(sessionCookie(c.Request()))
	if err != nil {
		return problem(c, http.StatusInternalServerError, "bootstrap_failed")
	}
	if setCookie {
		setSessionCookie(c, credentials.SessionID)
	}
	return c.JSON(http.StatusOK, api.BootstrapResponse{CsrfToken: credentials.CSRFToken})
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

func (h Handlers) Health(c *echo.Context) error {
	return c.JSON(http.StatusOK, api.HealthResponse{Status: "ok", Version: h.Version})
}
