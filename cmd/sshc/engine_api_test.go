package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"sshc/internal/api"
	"sshc/internal/handoff"
	"sshc/internal/httpserver"
	"sshc/internal/session"
)

const (
	engineAPISessionCanary = "engine-api-session-canary"
	engineAPICSRFCanary    = "engine-api-csrf-canary"
)

type engineAPIScript struct {
	t           *testing.T
	statusBody  string
	syncStatus  int
	syncBody    string
	sequence    []string
	sequenceMu  sync.Mutex
	sessionHits int
}

func (script *engineAPIScript) record(request *http.Request) {
	script.sequenceMu.Lock()
	defer script.sequenceMu.Unlock()
	script.sequence = append(script.sequence, request.Method+" "+request.URL.Path)
}

func (script *engineAPIScript) handler(response http.ResponseWriter, request *http.Request) {
	script.record(request)
	switch request.URL.Path {
	case httpserver.StatusPath:
		if request.Header.Get(handoff.HeaderName) != "the secret" {
			response.WriteHeader(http.StatusForbidden)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(response, script.statusBody)
	case httpserver.CLISessionPath:
		if request.Header.Get(handoff.HeaderName) != "the secret" {
			response.WriteHeader(http.StatusForbidden)
			return
		}
		if request.Method == http.MethodDelete {
			cookie, err := request.Cookie(httpserver.SessionCookie)
			if err != nil || cookie.Value != engineAPISessionCanary {
				script.t.Errorf("revoke cookie = %#v, %v", cookie, err)
			}
			response.WriteHeader(http.StatusNoContent)
			return
		}
		script.sessionHits++
		http.SetCookie(response, &http.Cookie{
			Name: httpserver.SessionCookie, Value: engineAPISessionCanary, Path: "/",
			HttpOnly: true, SameSite: http.SameSiteStrictMode,
		})
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(api.BootstrapResponse{CsrfToken: engineAPICSRFCanary})
	case "/api/v1/sync":
		cookie, err := request.Cookie(httpserver.SessionCookie)
		if err != nil || cookie.Value != engineAPISessionCanary {
			script.t.Errorf("API cookie = %#v, %v", cookie, err)
		}
		if got := request.Header.Get(httpserver.CSRFHeader); got != engineAPICSRFCanary {
			script.t.Errorf("CSRF = %q", got)
		}
		if got := request.Header.Get("Sec-Fetch-Site"); got != "same-origin" {
			script.t.Errorf("Sec-Fetch-Site = %q", got)
		}
		if request.Method != http.MethodGet && request.Header.Get("Origin") == "" {
			script.t.Error("mutation omitted Origin")
		}
		status := script.syncStatus
		if status == 0 {
			status = http.StatusOK
		}
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(status)
		_, _ = io.WriteString(response, script.syncBody)
	default:
		response.WriteHeader(http.StatusNotFound)
	}
}

func validEngineStatus() string {
	return `{"owner":"engine","version":"test","protocolVersion":1,"vault":true,"unlocked":true,"sessions":0}`
}

func openTestEngineAPI(t *testing.T, script *engineAPIScript) (*engineAPI, *httptest.Server, string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(script.handler))
	stateDir := t.TempDir()
	writeTestHandoff(t, stateDir, server.URL)
	opened, err := openEngineAPI(context.Background(), stateDir, server.Client())
	if err != nil {
		server.Close()
		t.Fatalf("openEngineAPI = %v", err)
	}
	return opened, server, stateDir
}

func TestEngineAPIAuthenticatesAndClosesTheCommandSession(t *testing.T) {
	script := &engineAPIScript{t: t, statusBody: validEngineStatus(), syncBody: `{"configured":true}`}
	server := httptest.NewServer(http.HandlerFunc(script.handler))
	defer server.Close()
	stateDir := t.TempDir()
	writeTestHandoff(t, stateDir, server.URL)

	base := server.Client()
	base.Timeout = 17 * time.Second
	opened, err := openEngineAPI(context.Background(), stateDir, base)
	if err != nil {
		t.Fatalf("openEngineAPI = %v", err)
	}
	if base.Timeout != 17*time.Second {
		t.Fatalf("caller client timeout changed to %s", base.Timeout)
	}
	var status api.SyncStatus
	if err := opened.getJSON(context.Background(), "/api/v1/sync", &status); err != nil {
		t.Fatalf("getJSON = %v", err)
	}
	if !status.Configured {
		t.Fatalf("sync status = %+v", status)
	}
	if err := opened.Close(); err != nil {
		t.Fatalf("Close = %v", err)
	}
	want := []string{
		"GET " + httpserver.StatusPath,
		"POST " + httpserver.CLISessionPath,
		"GET /api/v1/sync",
		"DELETE " + httpserver.CLISessionPath,
	}
	if strings.Join(script.sequence, "\n") != strings.Join(want, "\n") {
		t.Fatalf("request sequence = %q, want %q", script.sequence, want)
	}
}

func TestEngineAPIRejectsIdentityAndVaultMismatchesBeforeIssuingASession(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"owner", `{"owner":"desktop","version":"test","protocolVersion":1,"vault":true,"unlocked":true,"sessions":0}`},
		{"version", `{"owner":"engine","version":"other","protocolVersion":1,"vault":true,"unlocked":true,"sessions":0}`},
		{"protocol", `{"owner":"engine","version":"test","protocolVersion":2,"vault":true,"unlocked":true,"sessions":0}`},
		{"missing vault", `{"owner":"engine","version":"test","protocolVersion":1,"vault":false,"unlocked":false,"sessions":0}`},
		{"locked vault", `{"owner":"engine","version":"test","protocolVersion":1,"vault":true,"unlocked":false,"sessions":0}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			script := &engineAPIScript{t: t, statusBody: test.body}
			server := httptest.NewServer(http.HandlerFunc(script.handler))
			defer server.Close()
			stateDir := t.TempDir()
			writeTestHandoff(t, stateDir, server.URL)
			if opened, err := openEngineAPI(context.Background(), stateDir, server.Client()); err == nil || opened != nil {
				t.Fatalf("openEngineAPI = %#v, %v; want refusal", opened, err)
			}
			if script.sessionHits != 0 {
				t.Fatalf("CLI sessions issued = %d, want 0", script.sessionHits)
			}
		})
	}
}

func TestEngineAPIRefusesRedirectsAndInvalidBoundedJSON(t *testing.T) {
	t.Run("redirect", func(t *testing.T) {
		var redirected bool
		target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected = true }))
		defer target.Close()
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			http.Redirect(response, request, target.URL, http.StatusTemporaryRedirect)
		}))
		defer server.Close()
		stateDir := t.TempDir()
		writeTestHandoff(t, stateDir, server.URL)
		if _, err := openEngineAPI(context.Background(), stateDir, server.Client()); err == nil {
			t.Fatal("redirecting engine was accepted")
		}
		if redirected {
			t.Fatal("handoff secret followed a redirect")
		}
	})

	for _, test := range []struct {
		name string
		body string
	}{
		{"malformed", `{"owner":`},
		{"unknown field", `{"owner":"engine","version":"test","protocolVersion":1,"vault":true,"unlocked":true,"sessions":0,"surprise":true}`},
		{"trailing JSON", validEngineStatus() + `{}`},
		{"too large", strings.Repeat("x", maxEngineAPIResponse+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			script := &engineAPIScript{t: t, statusBody: test.body}
			server := httptest.NewServer(http.HandlerFunc(script.handler))
			defer server.Close()
			stateDir := t.TempDir()
			writeTestHandoff(t, stateDir, server.URL)
			if _, err := openEngineAPI(context.Background(), stateDir, server.Client()); err == nil {
				t.Fatal("invalid engine status was accepted")
			}
		})
	}
}

func TestEngineAPIHonorsCancellationAndDecodesProblemsWithoutLeakingTokens(t *testing.T) {
	t.Run("canceled open", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		stateDir := t.TempDir()
		writeTestHandoff(t, stateDir, "http://127.0.0.1:42888")
		_, err := openEngineAPI(ctx, stateDir, &http.Client{})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("openEngineAPI error = %v, want context.Canceled", err)
		}
	})

	t.Run("problem", func(t *testing.T) {
		problemBody := `{"code":"sync_conflict","message":"` + engineAPISessionCanary + ` ` + engineAPICSRFCanary + `"}`
		script := &engineAPIScript{
			t: t, statusBody: validEngineStatus(), syncStatus: http.StatusConflict, syncBody: problemBody,
		}
		opened, server, _ := openTestEngineAPI(t, script)
		defer server.Close()
		defer opened.Close()
		var status api.SyncStatus
		err := opened.getJSON(context.Background(), "/api/v1/sync", &status)
		var problem engineProblem
		if !errors.As(err, &problem) || problem.Status != http.StatusConflict ||
			problem.Code != "sync_conflict" || problem.Retryable || problem.OutcomeUnknown {
			t.Fatalf("problem = %#v, %v", problem, err)
		}
		if strings.Contains(err.Error(), engineAPISessionCanary) || strings.Contains(err.Error(), engineAPICSRFCanary) {
			t.Fatalf("problem leaked a token: %v", err)
		}
	})
}

type engineAPIRoundTripFunc func(*http.Request) (*http.Response, error)

func (function engineAPIRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestEngineAPIMutationTransportFailureIsOutcomeUnknownAndSecretPayloadIsErased(t *testing.T) {
	script := &engineAPIScript{t: t, statusBody: validEngineStatus()}
	server := httptest.NewServer(http.HandlerFunc(script.handler))
	defer server.Close()
	baseTransport := server.Client().Transport
	client := &http.Client{Transport: engineAPIRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/api/v1/sync/setup" {
			if request.GetBody != nil {
				t.Error("secret mutation body is replayable")
			}
			_, _ = io.ReadAll(request.Body)
			return nil, errors.New("transport reflected " + engineAPICSRFCanary)
		}
		return baseTransport.RoundTrip(request)
	})}
	stateDir := t.TempDir()
	writeTestHandoff(t, stateDir, server.URL)
	opened, err := openEngineAPI(context.Background(), stateDir, client)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()

	payload := []byte(`{"secretAccessKey":"a-secret"}`)
	err = opened.sendSecretJSON(context.Background(), http.MethodPut, "/api/v1/sync/setup", payload, nil)
	var problem engineProblem
	if !errors.As(err, &problem) || !problem.OutcomeUnknown || !problem.Retryable {
		t.Fatalf("mutation error = %#v, %v", problem, err)
	}
	if !allZero(payload) {
		t.Fatalf("secret payload was not erased: %q", payload)
	}
	if strings.Contains(err.Error(), engineAPICSRFCanary) {
		t.Fatalf("transport error leaked a token: %v", err)
	}
}

func TestEngineAPIIssueActionUsesTheTypedEndpoint(t *testing.T) {
	script := &engineAPIScript{t: t, statusBody: validEngineStatus()}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/actions" {
			script.handler(response, request)
			return
		}
		if request.Method != http.MethodPost || request.Header.Get("Origin") != serverOrigin(request) {
			t.Errorf("action headers: method=%s origin=%q", request.Method, request.Header.Get("Origin"))
		}
		var body api.IssueActionRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Kind != session.ActionSyncForcePush || body.Target != "etag-one" {
			t.Errorf("action request = %+v", body)
		}
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(response).Encode(api.IssueActionResponse{Token: "action-token", ExpiresAt: "soon"})
	}))
	defer server.Close()
	stateDir := t.TempDir()
	writeTestHandoff(t, stateDir, server.URL)
	opened, err := openEngineAPI(context.Background(), stateDir, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	issued, err := opened.issueAction(context.Background(), session.ActionSyncForcePush, "etag-one")
	if err != nil || issued.Token != "action-token" {
		t.Fatalf("issueAction = %+v, %v", issued, err)
	}
}

func serverOrigin(request *http.Request) string {
	return "http://" + request.Host
}
