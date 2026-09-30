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
	"sshc/internal/browserauth"
	"sshc/internal/session"
	"sshc/internal/storage"
)

const (
	theftTestHost   = "127.0.0.1:43123"
	theftTestOrigin = "http://" + theftTestHost
	theftTestProbe  = "/api/v1/probe-test"
)

// browserEntranceServer は、登録からの入り直しとサインアウトを本番と同じ経路で
// 通す。登録の猶予とセッションの期限は同じ時計で進める。
type browserEntranceServer struct {
	server    *echo.Echo
	bootstrap string
	now       time.Time
}

// browserVisit は、ブラウザが 1 回のリクエストで持っていくもの。
type browserVisit struct {
	browserToken string
	cookie       *http.Cookie
	csrf         string
}

func newBrowserEntranceServer(t *testing.T) *browserEntranceServer {
	t.Helper()
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	entrance := &browserEntranceServer{now: time.Unix(1_800_000_000, 0).UTC()}
	clock := func() time.Time { return entrance.now }
	registrations := browserauth.NewStore(workspace, rand.Reader).WithClock(clock)
	if err := registrations.SetPort(43123); err != nil {
		t.Fatal(err)
	}
	manager, bootstrap, err := session.NewManager(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manager.Now = clock
	entrance.bootstrap = bootstrap
	entrance.server = echo.New()
	entrance.server.Use((Security{
		ExpectedHost: theftTestHost, ExpectedOrigin: theftTestOrigin, Sessions: manager, Unlocked: alwaysUnlocked,
	}).Middleware)
	handlers := Handlers{Sessions: manager, BrowserAuth: registrations}
	entrance.server.POST("/api/v1/session/bootstrap", handlers.Bootstrap)
	entrance.server.POST("/api/v1/session/recover", handlers.Recover)
	entrance.server.POST("/api/v1/session/sign-out", handlers.SignOut)
	entrance.server.GET(theftTestProbe, func(c *echo.Context) error { return c.NoContent(http.StatusNoContent) })
	return entrance
}

func (e *browserEntranceServer) send(method, path string, visit browserVisit, extra http.Header) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, nil)
	request.Host = theftTestHost
	request.Header.Set(echo.HeaderOrigin, theftTestOrigin)
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.Header.Set("X-SSHC-Browser", visit.browserToken)
	request.Header.Set(CSRFHeader, visit.csrf)
	for name, values := range extra {
		request.Header[name] = values
	}
	if visit.cookie != nil {
		request.AddCookie(visit.cookie)
	}
	response := httptest.NewRecorder()
	e.server.ServeHTTP(response, request)
	return response
}

// enter は bootstrap か recover で入り、次のリクエストに持っていくものを返す。
// 既存のセッションに加わったときは、渡した cookie をそのまま使い続ける。
func (e *browserEntranceServer) enter(t *testing.T, path string, visit browserVisit, extra http.Header) browserVisit {
	t.Helper()
	response := e.send(http.MethodPost, path, visit, extra)
	if response.Code != http.StatusOK {
		t.Fatalf("POST %s = %d: %s", path, response.Code, response.Body.String())
	}
	var entered api.BootstrapResponse
	if err := json.Unmarshal(response.Body.Bytes(), &entered); err != nil {
		t.Fatal(err)
	}
	next := browserVisit{browserToken: visit.browserToken, cookie: visit.cookie, csrf: entered.CsrfToken}
	if entered.BrowserToken != nil {
		next.browserToken = *entered.BrowserToken
	}
	if cookies := response.Result().Cookies(); len(cookies) > 0 {
		next.cookie = cookies[0]
	}
	return next
}

func (e *browserEntranceServer) probe(visit browserVisit) int {
	return e.send(http.MethodGet, theftTestProbe, visit, nil).Code
}

// 複製された登録 token で先に入り直した側のセッションは、正規のブラウザが古い
// token を出して複製が見つかった時点で失効する。登録だけを消すと、複製した側は
// セッションの期限まで API を使い続けられる。
func TestTheCopiersSessionEndsWhenTheCopiedRegistrationIsDetected(t *testing.T) {
	entrance := newBrowserEntranceServer(t)
	genuine := entrance.enter(t, "/api/v1/session/bootstrap", browserVisit{},
		http.Header{"X-Sshc-Bootstrap": {entrance.bootstrap}})
	copier := entrance.enter(t, "/api/v1/session/recover", browserVisit{browserToken: genuine.browserToken}, nil)
	if code := entrance.probe(copier); code != http.StatusNoContent {
		t.Fatalf("the copier's session before detection = %d", code)
	}

	entrance.now = entrance.now.Add(2 * time.Minute)
	detected := entrance.send(http.MethodPost, "/api/v1/session/recover", browserVisit{browserToken: genuine.browserToken}, nil)
	if detected.Code != http.StatusUnauthorized {
		t.Fatalf("recover with the retired token = %d, want the copy detected", detected.Code)
	}
	if code := entrance.probe(copier); code != http.StatusUnauthorized {
		t.Fatalf("the copier's session after detection = %d, want it revoked", code)
	}
}

// 登録を添えてサインアウトしたら、同じ登録から入ったほかのセッションも失効する。
func TestSigningOutWithTheRegistrationEndsEverySessionItEntered(t *testing.T) {
	entrance := newBrowserEntranceServer(t)
	genuine := entrance.enter(t, "/api/v1/session/bootstrap", browserVisit{},
		http.Header{"X-Sshc-Bootstrap": {entrance.bootstrap}})
	copier := entrance.enter(t, "/api/v1/session/recover", browserVisit{browserToken: genuine.browserToken}, nil)
	// 猶予のうちに戻った正規のブラウザは、複製した側と同じ新しい token を受け取る。
	entrance.now = entrance.now.Add(30 * time.Second)
	genuine = entrance.enter(t, "/api/v1/session/recover", genuine, nil)

	signedOut := entrance.send(http.MethodPost, "/api/v1/session/sign-out", genuine, nil)
	if signedOut.Code != http.StatusNoContent {
		t.Fatalf("sign-out = %d: %s", signedOut.Code, signedOut.Body.String())
	}
	if code := entrance.probe(copier); code != http.StatusUnauthorized {
		t.Fatalf("the copier's session after sign-out = %d, want it revoked", code)
	}
}
