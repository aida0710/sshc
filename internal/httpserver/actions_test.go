package httpserver

import (
	"context"
	"crypto/rand"
	"net/http"
	"testing"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/session"
)

// 確認を出せないときの拒否は、それだけで応答を終える。拒否を書いたあとに
// 成功の本文を書き足すと本文が JSON 2 つの連結になり、ブラウザは拒否の code を
// 読めなくなる。
func TestIssueActionAtTheTokenLimitAnswersWithOnlyTheRefusal(t *testing.T) {
	// 乱数を固定するとトークンが毎回同じになり、表が埋まらない。
	manager, bootstrap, err := session.NewManager(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	credentials, _, err := manager.BootstrapForSession(bootstrap, "")
	if err != nil {
		t.Fatal(err)
	}
	engine := echo.New()
	engine.Use((Security{
		ExpectedHost: keyTestHost, ExpectedOrigin: "http://" + keyTestHost,
		Sessions: manager, Unlocked: alwaysUnlocked,
	}).Middleware)
	registerActionRoutes(engine, ActionHandlers{Sessions: manager, Kinds: actionRegistry{
		session.ActionReachability: {evidence: func(context.Context, string) (string, error) { return "evidence", nil }},
	}})
	body := mustMarshal(t, api.IssueActionRequest{Kind: session.ActionReachability, Target: "bastion"})

	for range session.MaxActionTokensPerSession {
		response := sendKeyRequest(t, engine, credentials, http.MethodPost, "/api/v1/actions", body, "")
		if response.Code != http.StatusCreated {
			t.Fatalf("issue action = %d: %s", response.Code, response.Body.String())
		}
	}
	refused := sendKeyRequest(t, engine, credentials, http.MethodPost, "/api/v1/actions", body, "")
	if refused.Code != http.StatusTooManyRequests {
		t.Fatalf("issue action past the limit = %d: %s", refused.Code, refused.Body.String())
	}
	// problemCode は本文全体を 1 つの JSON として読むので、書き足された本文があれば失敗する。
	if code := problemCode(t, refused.Body.Bytes()); code != "too_many_confirmations" {
		t.Errorf("code = %q", code)
	}
}
