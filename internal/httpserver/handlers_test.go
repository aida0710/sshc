package httpserver

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/browserauth"
	"sshc/internal/session"
	"sshc/internal/storage"
)

func TestBootstrapHandlerSetsStrictSessionCookieAndRejectsReplay(t *testing.T) {
	manager, bootstrap, err := session.NewManager(bytes.NewReader(bytes.Repeat([]byte{0x61}, 96)))
	if err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Use((Security{
		ExpectedHost:   "127.0.0.1:43123",
		ExpectedOrigin: "http://127.0.0.1:43123",
		Sessions:       manager, Unlocked: alwaysUnlocked,
	}).Middleware)
	e.POST("/api/v1/session/bootstrap", (Handlers{Sessions: manager, Version: "test-version"}).Bootstrap)

	call := func(token string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/session/bootstrap", nil)
		request.Host = "127.0.0.1:43123"
		request.Header.Set(echo.HeaderOrigin, "http://127.0.0.1:43123")
		request.Header.Set("Sec-Fetch-Site", "same-origin")
		request.Header.Set("X-SSHC-Bootstrap", token)
		response := httptest.NewRecorder()
		e.ServeHTTP(response, request)
		return response
	}

	response := call(bootstrap)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	result := response.Result()
	cookies := result.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %#v", cookies)
	}
	cookie := cookies[0]
	if cookie.Name != SessionCookie || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" || cookie.Secure {
		t.Fatalf("cookie = %#v", cookie)
	}
	if got := result.Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("cache control = %q", got)
	}
	var payload api.BootstrapResponse
	if err := json.NewDecoder(result.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.CsrfToken) != 43 {
		t.Fatalf("csrf length = %d", len(payload.CsrfToken))
	}
	if got := call(bootstrap).Code; got != http.StatusConflict {
		t.Fatalf("replay = %d, want %d", got, http.StatusConflict)
	}
	invalid := call("wrong-token")
	if invalid.Code != http.StatusConflict {
		t.Fatalf("invalid after bootstrap = %d, want %d", invalid.Code, http.StatusConflict)
	}
}

func TestRegisteredBrowserRecoversWithoutReplacingAValidSessionCookie(t *testing.T) {
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	random := bytes.NewReader(bytes.Repeat([]byte{0x74}, 512))
	// The store needs distinct bytes per mint: a recovery must hand out a token
	// that differs from the one it retires.
	registrations := browserauth.NewStore(workspace, rand.Reader)
	if err := registrations.SetPort(43123); err != nil {
		t.Fatal(err)
	}
	manager, bootstrap, err := session.NewManager(random)
	if err != nil {
		t.Fatal(err)
	}
	engine := func(manager *session.Manager) *echo.Echo {
		e := echo.New()
		e.Use((Security{
			ExpectedHost: "127.0.0.1:43123", ExpectedOrigin: "http://127.0.0.1:43123",
			Sessions: manager, Unlocked: alwaysUnlocked,
		}).Middleware)
		handlers := Handlers{Sessions: manager, BrowserAuth: registrations}
		e.POST("/api/v1/session/bootstrap", handlers.Bootstrap)
		e.POST("/api/v1/session/recover", handlers.Recover)
		return e
	}
	call := func(e *echo.Echo, path string, cookie *http.Cookie, headers map[string]string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, path, nil)
		request.Host = "127.0.0.1:43123"
		request.Header.Set(echo.HeaderOrigin, "http://127.0.0.1:43123")
		request.Header.Set("Sec-Fetch-Site", "same-origin")
		if cookie != nil {
			request.AddCookie(cookie)
		}
		for name, value := range headers {
			request.Header.Set(name, value)
		}
		response := httptest.NewRecorder()
		e.ServeHTTP(response, request)
		return response
	}

	first := call(engine(manager), "/api/v1/session/bootstrap", nil, map[string]string{"X-SSHC-Bootstrap": bootstrap})
	if first.Code != http.StatusOK || len(first.Result().Cookies()) != 1 {
		t.Fatalf("bootstrap status=%d cookies=%#v", first.Code, first.Result().Cookies())
	}
	var established api.BootstrapResponse
	if err := json.NewDecoder(first.Body).Decode(&established); err != nil {
		t.Fatal(err)
	}
	if established.BrowserToken == nil || len(*established.BrowserToken) != 43 {
		t.Fatalf("browser token = %#v", established.BrowserToken)
	}
	cookie := first.Result().Cookies()[0]
	joined := call(engine(manager), "/api/v1/session/recover", cookie, map[string]string{"X-SSHC-Browser": *established.BrowserToken})
	if joined.Code != http.StatusOK || len(joined.Result().Cookies()) != 0 {
		t.Fatalf("joined status=%d cookies=%#v", joined.Code, joined.Result().Cookies())
	}

	restarted, _, err := session.NewManager(bytes.NewReader(bytes.Repeat([]byte{0x75}, 256)))
	if err != nil {
		t.Fatal(err)
	}
	recovered := call(engine(restarted), "/api/v1/session/recover", cookie, map[string]string{"X-SSHC-Browser": *established.BrowserToken})
	if recovered.Code != http.StatusOK || len(recovered.Result().Cookies()) != 1 || recovered.Result().Cookies()[0].Value == cookie.Value {
		t.Fatalf("recovered status=%d cookies=%#v", recovered.Code, recovered.Result().Cookies())
	}
	// Every recovery hands out a replacement registration. The browser stores it
	// and the presented one retires once the grace for other tabs has passed.
	var rotated api.BootstrapResponse
	if err := json.NewDecoder(recovered.Body).Decode(&rotated); err != nil {
		t.Fatal(err)
	}
	if rotated.BrowserToken == nil || len(*rotated.BrowserToken) != 43 || *rotated.BrowserToken == *established.BrowserToken {
		t.Fatalf("rotated browser token = %#v", rotated.BrowserToken)
	}
	denied := call(engine(restarted), "/api/v1/session/recover", nil, map[string]string{"X-SSHC-Browser": "x"})
	if denied.Code != http.StatusUnauthorized || len(denied.Result().Cookies()) != 0 {
		t.Fatalf("invalid registration status=%d cookies=%#v", denied.Code, denied.Result().Cookies())
	}
	if err := registrations.SetPort(44123); err != nil {
		t.Fatal(err)
	}
	moved := call(engine(restarted), "/api/v1/session/recover", nil, map[string]string{"X-SSHC-Browser": *rotated.BrowserToken})
	if moved.Code != http.StatusUnauthorized || len(moved.Result().Cookies()) != 0 {
		t.Fatalf("previous-port registration status=%d cookies=%#v", moved.Code, moved.Result().Cookies())
	}
}

func TestBootstrapHandlerRejectsWrongTokenWithoutCookie(t *testing.T) {
	manager, _, err := session.NewManager(bytes.NewReader(bytes.Repeat([]byte{0x63}, 96)))
	if err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Use((Security{
		ExpectedHost:   "127.0.0.1:43123",
		ExpectedOrigin: "http://127.0.0.1:43123",
		Sessions:       manager, Unlocked: alwaysUnlocked,
	}).Middleware)
	e.POST("/api/v1/session/bootstrap", (Handlers{Sessions: manager}).Bootstrap)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/session/bootstrap", nil)
	request.Host = "127.0.0.1:43123"
	request.Header.Set(echo.HeaderOrigin, "http://127.0.0.1:43123")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.Header.Set("X-SSHC-Bootstrap", "wrong-token")
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	if cookies := response.Result().Cookies(); len(cookies) != 0 {
		t.Fatalf("cookies = %#v", cookies)
	}
}

func TestHealthRequiresSessionCookie(t *testing.T) {
	manager, bootstrap, err := session.NewManager(bytes.NewReader(bytes.Repeat([]byte{0x71}, 96)))
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := manager.Bootstrap(bootstrap)
	if err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Use((Security{
		ExpectedHost:   "127.0.0.1:43123",
		ExpectedOrigin: "http://127.0.0.1:43123",
		Sessions:       manager, Unlocked: alwaysUnlocked,
	}).Middleware)
	e.GET("/api/v1/health", (Handlers{Sessions: manager, Version: "test-version"}).Health)

	call := func(authenticated bool) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
		request.Host = "127.0.0.1:43123"
		request.Header.Set("Sec-Fetch-Site", "same-origin")
		if authenticated {
			request.AddCookie(&http.Cookie{Name: SessionCookie, Value: credentials.SessionID})
		}
		response := httptest.NewRecorder()
		e.ServeHTTP(response, request)
		return response
	}

	if got := call(false).Code; got != http.StatusUnauthorized {
		t.Fatalf("without cookie = %d, want %d", got, http.StatusUnauthorized)
	}
	response := call(true)
	if response.Code != http.StatusOK {
		t.Fatalf("with cookie = %d, want %d", response.Code, http.StatusOK)
	}
	if got := strings.TrimSpace(response.Body.String()); got != `{"status":"ok","version":"test-version"}` {
		t.Fatalf("body = %s", got)
	}
}

func TestSignOutRevokesTheSessionAndTheRegistrationItNames(t *testing.T) {
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	registrations := browserauth.NewStore(workspace, rand.Reader)
	if err := registrations.SetPort(43123); err != nil {
		t.Fatal(err)
	}
	manager, bootstrap, err := session.NewManager(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Use((Security{
		ExpectedHost: "127.0.0.1:43123", ExpectedOrigin: "http://127.0.0.1:43123",
		Sessions: manager, Unlocked: alwaysUnlocked,
	}).Middleware)
	handlers := Handlers{Sessions: manager, BrowserAuth: registrations}
	e.POST("/api/v1/session/bootstrap", handlers.Bootstrap)
	e.POST("/api/v1/session/recover", handlers.Recover)
	e.POST("/api/v1/session/sign-out", handlers.SignOut)
	e.GET("/api/v1/health", handlers.Health)
	call := func(method, path string, cookie *http.Cookie, headers map[string]string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, nil)
		request.Host = "127.0.0.1:43123"
		request.Header.Set(echo.HeaderOrigin, "http://127.0.0.1:43123")
		request.Header.Set("Sec-Fetch-Site", "same-origin")
		if cookie != nil {
			request.AddCookie(cookie)
		}
		for name, value := range headers {
			request.Header.Set(name, value)
		}
		response := httptest.NewRecorder()
		e.ServeHTTP(response, request)
		return response
	}

	entered := call(http.MethodPost, "/api/v1/session/bootstrap", nil, map[string]string{"X-SSHC-Bootstrap": bootstrap})
	if entered.Code != http.StatusOK {
		t.Fatalf("bootstrap status=%d", entered.Code)
	}
	var established api.BootstrapResponse
	if err := json.NewDecoder(entered.Body).Decode(&established); err != nil {
		t.Fatal(err)
	}
	cookie := entered.Result().Cookies()[0]
	authenticated := map[string]string{CSRFHeader: established.CsrfToken, "X-SSHC-Browser": *established.BrowserToken}

	// Without the CSRF token the request is refused like any other mutation.
	if refused := call(http.MethodPost, "/api/v1/session/sign-out", cookie, nil); refused.Code != http.StatusForbidden {
		t.Fatalf("sign-out without CSRF status=%d, want 403", refused.Code)
	}
	signedOut := call(http.MethodPost, "/api/v1/session/sign-out", cookie, authenticated)
	if signedOut.Code != http.StatusNoContent {
		t.Fatalf("sign-out status=%d body=%s", signedOut.Code, signedOut.Body.String())
	}
	cleared := signedOut.Result().Cookies()
	if len(cleared) != 1 || cleared[0].Name != SessionCookie || cleared[0].MaxAge >= 0 {
		t.Fatalf("sign-out cookies=%#v, want the session cookie cleared", cleared)
	}
	if after := call(http.MethodGet, "/api/v1/health", cookie, map[string]string{CSRFHeader: established.CsrfToken}); after.Code != http.StatusUnauthorized {
		t.Fatalf("health after sign-out status=%d, want 401", after.Code)
	}
	if recovered := call(http.MethodPost, "/api/v1/session/recover", nil, map[string]string{"X-SSHC-Browser": *established.BrowserToken}); recovered.Code != http.StatusUnauthorized {
		t.Fatalf("recover after sign-out status=%d, want 401", recovered.Code)
	}
}
