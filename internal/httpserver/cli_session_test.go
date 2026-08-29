package httpserver

import (
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/handoff"
	"sshc/internal/session"
)

const (
	cliSessionTestHost   = "127.0.0.1:4242"
	cliSessionTestOrigin = "http://127.0.0.1:4242"
	cliSessionTestSecret = "handoff-secret-for-cli-session-tests"
)

func newCLISessionEngine(t *testing.T) (*echo.Echo, *session.Manager, string) {
	t.Helper()
	manager, bootstrap, err := session.NewManager(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	engine := echo.New()
	engine.Use((Security{
		ExpectedHost:   cliSessionTestHost,
		ExpectedOrigin: cliSessionTestOrigin,
		Sessions:       manager,
		Unlocked:       func() bool { return true },
	}).Middleware)
	registerConnectRoutes(engine, ConnectHandlers{
		Secret:    cliSessionTestSecret,
		Bootstrap: manager,
	})
	engine.GET("/api/v1/protected-test", func(c *echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	})
	return engine, manager, bootstrap
}

func requestCLISession(t *testing.T, engine *echo.Echo, secret string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, CLISessionPath, nil)
	request.Host = cliSessionTestHost
	if secret != "" {
		request.Header.Set(handoff.HeaderName, secret)
	}
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	return response
}

func issuedCLISession(t *testing.T, engine *echo.Echo) (*http.Cookie, string) {
	t.Helper()
	response := requestCLISession(t, engine, cliSessionTestSecret)
	if response.Code != http.StatusOK {
		t.Fatalf("POST %s = %d: %s", CLISessionPath, response.Code, response.Body.String())
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("session cookies = %d, want 1", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != SessionCookie || !cookie.HttpOnly || cookie.Path != "/" ||
		cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie = %+v", cookie)
	}
	var answer api.BootstrapResponse
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if answer.CsrfToken == "" {
		t.Fatal("CLI session response did not include a CSRF token")
	}
	return cookie, answer.CsrfToken
}

func protectedCLIRequest(engine *echo.Echo, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/protected-test", nil)
	request.Host = cliSessionTestHost
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.Header.Set(CSRFHeader, csrf)
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	return response
}

func TestCLISessionRequiresHandoffAndUsesNormalAPISecurity(t *testing.T) {
	engine, manager, _ := newCLISessionEngine(t)

	missing := requestCLISession(t, engine, "")
	wrong := requestCLISession(t, engine, "wrong-secret")
	if missing.Code != http.StatusForbidden || wrong.Code != missing.Code ||
		wrong.Body.String() != missing.Body.String() {
		t.Fatalf("handoff refusals differ: missing=%d %q wrong=%d %q",
			missing.Code, missing.Body.String(), wrong.Code, wrong.Body.String())
	}

	cookie, csrf := issuedCLISession(t, engine)
	if !manager.VerifyCSRF(cookie.Value, csrf) {
		t.Fatal("CLI CSRF token was not registered")
	}
	if response := protectedCLIRequest(engine, cookie, csrf); response.Code != http.StatusNoContent {
		t.Fatalf("protected API with CLI session = %d: %s", response.Code, response.Body.String())
	}
	if response := protectedCLIRequest(engine, cookie, "wrong-csrf"); response.Code != http.StatusForbidden {
		t.Fatalf("protected API with wrong CSRF = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestCLISessionRevokeAndHardExpiry(t *testing.T) {
	engine, manager, _ := newCLISessionEngine(t)
	now := time.Unix(1_800_000_000, 0).UTC()
	manager.Now = func() time.Time { return now }

	cookie, csrf := issuedCLISession(t, engine)
	revoke := httptest.NewRequest(http.MethodDelete, CLISessionPath, nil)
	revoke.Host = cliSessionTestHost
	revoke.Header.Set(handoff.HeaderName, cliSessionTestSecret)
	revoke.AddCookie(cookie)
	revoked := httptest.NewRecorder()
	engine.ServeHTTP(revoked, revoke)
	if revoked.Code != http.StatusNoContent {
		t.Fatalf("DELETE %s = %d: %s", CLISessionPath, revoked.Code, revoked.Body.String())
	}
	if response := protectedCLIRequest(engine, cookie, csrf); response.Code != http.StatusUnauthorized {
		t.Fatalf("revoked CLI session API response = %d, want %d", response.Code, http.StatusUnauthorized)
	}

	expiringCookie, expiringCSRF := issuedCLISession(t, engine)
	now = now.Add(CLISessionTTL)
	if response := protectedCLIRequest(engine, expiringCookie, expiringCSRF); response.Code != http.StatusUnauthorized {
		t.Fatalf("expired CLI session API response = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestCLISessionDoesNotConsumeBrowserBootstrap(t *testing.T) {
	engine, manager, bootstrap := newCLISessionEngine(t)
	issuedCLISession(t, engine)

	credentials, err := manager.Bootstrap(bootstrap)
	if err != nil {
		t.Fatalf("browser Bootstrap after CLI session = %v", err)
	}
	if !manager.Authenticate(credentials.SessionID) {
		t.Fatal("browser bootstrap did not create a session")
	}
}
