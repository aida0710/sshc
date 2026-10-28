package httpserver

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/sshclient"
)

func destinationEngine(resolve func(string) (sshclient.Target, error)) *echo.Echo {
	engine := echo.New()
	registerTerminalRoutes(engine, TerminalHandlers{ResolveTarget: resolve})
	return engine
}

func getDestination(engine *echo.Echo, alias string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/terminal/destinations/"+alias, nil))
	return recorder
}

func TestADestinationIsWhereTheConfigurationTakesTheAliasWithoutConnecting(t *testing.T) {
	engine := destinationEngine(func(alias string) (sshclient.Target, error) {
		return sshclient.Target{Alias: alias, HostName: "203.0.113.10", Port: "2222", User: "ops"}, nil
	})
	recorder := getDestination(engine, "bastion")
	if recorder.Code != http.StatusOK {
		t.Fatalf("destination = %d %s", recorder.Code, recorder.Body.String())
	}
	got := decodeAs[api.TerminalDestination](t, recorder.Body.String())
	if got != (api.TerminalDestination{Alias: "bastion", HostName: "203.0.113.10", Port: "2222", User: "ops"}) {
		t.Fatalf("destination = %+v", got)
	}
}

func TestADestinationThatCannotBeResolvedIsRefusedWithoutTheResolverText(t *testing.T) {
	engine := destinationEngine(func(string) (sshclient.Target, error) {
		return sshclient.Target{}, errors.New("ProxyCommand secret-token failed")
	})
	recorder := getDestination(engine, "bastion")
	problem := decodeAs[api.Problem](t, recorder.Body.String())
	if recorder.Code != http.StatusUnprocessableEntity || problem.Code != "alias_unresolvable" {
		t.Fatalf("unresolvable = %d %s", recorder.Code, recorder.Body.String())
	}
	if recorder := getDestination(engine, "-oProxyCommand=id"); recorder.Code != http.StatusBadRequest {
		t.Fatalf("unsafe alias = %d %s", recorder.Code, recorder.Body.String())
	}
}
