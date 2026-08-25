package httpserver

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/session"
)

type Handlers struct {
	Sessions *session.Manager
	Version  string
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

	credentials, err := h.Sessions.Bootstrap(c.Request().Header.Get("X-SSHC-Bootstrap"))
	switch {
	case errors.Is(err, session.ErrInvalidBootstrap):
		return problem(c, http.StatusUnauthorized, "invalid_bootstrap")
	case errors.Is(err, session.ErrBootstrapUsed):
		return problem(c, http.StatusConflict, "bootstrap_used")
	case err != nil:
		return problem(c, http.StatusInternalServerError, "bootstrap_failed")
	}

	c.SetCookie(&http.Cookie{
		Name:     SessionCookie,
		Value:    credentials.SessionID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	return c.JSON(http.StatusOK, api.BootstrapResponse{CsrfToken: credentials.CSRFToken})
}

func (h Handlers) Health(c *echo.Context) error {
	return c.JSON(http.StatusOK, api.HealthResponse{Status: "ok", Version: h.Version})
}
