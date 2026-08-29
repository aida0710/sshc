package httpserver

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
)

const (
	// CLISessionPath exchanges the user-private handoff secret for a short-lived
	// normal API session. Sync commands can therefore reuse the browser API.
	CLISessionPath = "/cli/session"
	// CLISessionTTL bounds a credential left behind when a command is killed
	// before it can revoke the session.
	CLISessionTTL = 10 * time.Minute
)

// CLISession issues a short-lived normal API cookie and its origin-scoped CSRF
// token without consuming or replacing the browser bootstrap token.
func (h ConnectHandlers) CLISession(c *echo.Context) error {
	if !cliAuthorised(c.Request(), h.Secret) {
		return c.NoContent(http.StatusForbidden)
	}
	if h.Bootstrap == nil {
		return c.NoContent(http.StatusServiceUnavailable)
	}
	credentials, err := h.Bootstrap.IssueExpiring(CLISessionTTL)
	if err != nil {
		return c.NoContent(http.StatusInternalServerError)
	}
	setSessionCookie(c, credentials.SessionID)
	return c.JSON(http.StatusOK, api.BootstrapResponse{CsrfToken: credentials.CSRFToken})
}

// RevokeCLISession revokes only the session named by the request cookie. A
// missing cookie is already the desired state, so revocation is idempotent.
func (h ConnectHandlers) RevokeCLISession(c *echo.Context) error {
	if !cliAuthorised(c.Request(), h.Secret) {
		return c.NoContent(http.StatusForbidden)
	}
	if h.Bootstrap == nil {
		return c.NoContent(http.StatusServiceUnavailable)
	}
	if cookie, err := c.Request().Cookie(SessionCookie); err == nil {
		h.Bootstrap.Revoke(cookie.Value)
	}
	return c.NoContent(http.StatusNoContent)
}
