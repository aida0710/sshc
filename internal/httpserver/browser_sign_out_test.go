package httpserver

import (
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/browserauth"
	"sshc/internal/session"
	"sshc/internal/storage"
)

func TestSignedOutBrowserCannotRecoverUsingAnOlderGraceToken(t *testing.T) {
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
	server := echo.New()
	server.Use((Security{ExpectedHost: "127.0.0.1:43123", ExpectedOrigin: "http://127.0.0.1:43123", Sessions: manager, Unlocked: alwaysUnlocked}).Middleware)
	handlers := Handlers{Sessions: manager, BrowserAuth: registrations}
	server.POST("/api/v1/session/bootstrap", handlers.Bootstrap)
	server.POST("/api/v1/session/recover", handlers.Recover)
	server.POST("/api/v1/session/sign-out", handlers.SignOut)
	type browserRequest struct {
		path           string
		browserToken   string
		bootstrapToken string
		csrf           string
		cookie         *http.Cookie
	}
	call := func(options browserRequest) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, options.path, nil)
		request.Host = "127.0.0.1:43123"
		request.Header.Set(echo.HeaderOrigin, "http://127.0.0.1:43123")
		request.Header.Set("Sec-Fetch-Site", "same-origin")
		request.Header.Set("X-SSHC-Bootstrap", options.bootstrapToken)
		request.Header.Set("X-SSHC-Browser", options.browserToken)
		request.Header.Set(CSRFHeader, options.csrf)
		if options.cookie != nil {
			request.AddCookie(options.cookie)
		}
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		return response
	}
	decode := func(response *httptest.ResponseRecorder) api.BootstrapResponse {
		if response.Code != http.StatusOK {
			t.Fatalf("session request failed: %d", response.Code)
		}
		var credentials api.BootstrapResponse
		if err := json.NewDecoder(response.Body).Decode(&credentials); err != nil {
			t.Fatal(err)
		}
		if credentials.BrowserToken == nil {
			t.Fatal("missing browser token")
		}
		return credentials
	}
	entered := call(browserRequest{path: "/api/v1/session/bootstrap", bootstrapToken: bootstrap})
	first := decode(entered)
	cookie := entered.Result().Cookies()[0]
	second := decode(call(browserRequest{path: "/api/v1/session/recover", browserToken: *first.BrowserToken, cookie: cookie}))
	third := decode(call(browserRequest{path: "/api/v1/session/recover", browserToken: *second.BrowserToken, cookie: cookie}))
	signedOut := call(browserRequest{path: "/api/v1/session/sign-out", browserToken: *third.BrowserToken, cookie: cookie, csrf: third.CsrfToken})
	if signedOut.Code != http.StatusNoContent {
		t.Fatalf("sign-out failed: %d", signedOut.Code)
	}
	if registered, err := registrations.HasRegistrations(); err != nil || registered {
		t.Fatalf("registration was not removed: %v", err)
	}
	reentered := call(browserRequest{path: "/api/v1/session/recover", browserToken: *first.BrowserToken})
	if reentered.Code == http.StatusOK && len(reentered.Result().Cookies()) == 1 {
		t.Fatal("revoked browser registration issued a fresh session cookie through an older grace token")
	}
	if reentered.Code != http.StatusUnauthorized {
		t.Fatalf("recover status: %d", reentered.Code)
	}
}
