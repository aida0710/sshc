package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sshc/internal/api"
	"sshc/internal/handoff"
	"sshc/internal/httpserver"
)

func otpCommandServer(t *testing.T) (*httptest.Server, *bool) {
	t.Helper()
	closed := false
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case httpserver.StatusPath:
			if request.Header.Get(handoff.HeaderName) != "the secret" {
				response.WriteHeader(http.StatusForbidden)
				return
			}
			_, _ = io.WriteString(response, validEngineStatus())
		case httpserver.CLISessionPath:
			if request.Method == http.MethodDelete {
				closed = true
				response.WriteHeader(http.StatusNoContent)
				return
			}
			http.SetCookie(response, &http.Cookie{
				Name: httpserver.SessionCookie, Value: engineAPISessionCanary, Path: "/",
			})
			_ = json.NewEncoder(response).Encode(api.BootstrapResponse{CsrfToken: engineAPICSRFCanary})
		case "/api/v1/credentials":
			_ = json.NewEncoder(response).Encode(api.CredentialList{Credentials: []api.Credential{
				{Kind: api.Totp, Name: "production", Hosts: []string{"bastion"}},
				{Kind: api.Password, Name: "not-an-otp"},
			}})
		case "/api/v1/actions":
			var issued api.IssueActionRequest
			if err := json.NewDecoder(request.Body).Decode(&issued); err != nil {
				t.Errorf("decode action: %v", err)
			}
			if issued.Kind != "credential.reveal" || issued.Target != "totp\nproduction" {
				t.Errorf("action = %#v", issued)
			}
			_ = json.NewEncoder(response).Encode(api.IssueActionResponse{
				Token: "otp-action", ExpiresAt: "2026-09-08T00:01:00Z",
			})
		case "/api/v1/credentials/totp/production/codes":
			if request.Header.Get(httpserver.ActionHeader) != "otp-action" {
				response.WriteHeader(http.StatusForbidden)
				return
			}
			_ = json.NewEncoder(response).Encode(api.TOTPCodeSet{
				Previous: "111111", Current: "222222", Next: "333333",
				PeriodSeconds: 30, RemainingSeconds: 17,
			})
		default:
			response.WriteHeader(http.StatusNotFound)
		}
	}))
	return server, &closed
}

func TestOTPCLIListsNamesWithoutCodesOrProvisioningData(t *testing.T) {
	server, closed := otpCommandServer(t)
	defer server.Close()
	stateDir := t.TempDir()
	writeTestHandoff(t, stateDir, server.URL)
	var stdout, stderr bytes.Buffer

	code := runOTP(context.Background(), otpInvocation{Action: otpList}, stateDir,
		server.Client(), nil, &stdout, &stderr, nil)
	if code != 0 || stderr.Len() != 0 || !*closed {
		t.Fatalf("runOTP = %d, stdout %q, stderr %q, closed %v", code, stdout.String(), stderr.String(), *closed)
	}
	if got := stdout.String(); !strings.Contains(got, "production\tbastion") ||
		strings.Contains(got, "222222") || strings.Contains(got, "JBSWY3DPEHPK3PXP") {
		t.Fatalf("list output = %q", got)
	}
}

func TestOTPCLIShowsAdjacentCodesWithoutProvisioningData(t *testing.T) {
	server, closed := otpCommandServer(t)
	defer server.Close()
	stateDir := t.TempDir()
	writeTestHandoff(t, stateDir, server.URL)
	var stdout, stderr bytes.Buffer

	code := runOTP(context.Background(), otpInvocation{Action: otpShow, Name: "production"},
		stateDir, server.Client(), nil, &stdout, &stderr, nil)
	if code != 0 || stderr.Len() != 0 || !*closed {
		t.Fatalf("runOTP = %d, stdout %q, stderr %q, closed %v", code, stdout.String(), stderr.String(), *closed)
	}
	for _, wanted := range []string{"previous  111111", "current   222222  (17s remaining)", "next      333333"} {
		if !strings.Contains(stdout.String(), wanted) {
			t.Errorf("output lacks %q: %s", wanted, stdout.String())
		}
	}
	if strings.Contains(stdout.String(), "JBSWY3DPEHPK3PXP") {
		t.Fatalf("show output exposed setup key: %s", stdout.String())
	}
}
