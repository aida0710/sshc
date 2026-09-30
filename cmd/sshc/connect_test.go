package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"sshc/internal/handoff"
	"sshc/internal/httpserver"
)

func TestRequestConnectionSendsTheAliasWithTheHandoffSecretAndReadsTheAnswer(t *testing.T) {
	server := engineTestServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != httpserver.ConnectPath {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get(handoff.HeaderName) != "the secret" {
			t.Errorf("handoff secret = %q", request.Header.Get(handoff.HeaderName))
		}
		if request.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content type = %q", request.Header.Get("Content-Type"))
		}
		var body map[string]string
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil || body["alias"] != "web" {
			t.Errorf("body = %#v, %v", body, err)
		}
		_, _ = io.WriteString(response, `{"alias":"web","passwords":{"web":"pw"},"warnings":[]}`+"\n")
	}))
	defer server.Close()

	answer, err := requestConnection(context.Background(), testHandoff(server.URL), "web", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if answer.Alias != "web" || answer.Passwords["web"] != "pw" {
		t.Fatalf("answer = %#v", answer)
	}
}

func TestRunOpenPrintsTheURLFromTheEngine(t *testing.T) {
	server := engineTestServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != httpserver.OpenPath {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		_, _ = io.WriteString(response, `{"url":"http://127.0.0.1:1/#bootstrap=token"}`)
	}))
	defer server.Close()
	stateDir := t.TempDir()
	writeTestHandoff(t, stateDir, server.URL)

	var stdout, stderr bytes.Buffer
	environment := commandEnvironment{stateDir: stateDir, client: server.Client(), stdout: &stdout, stderr: &stderr}
	if code := runOpen(context.Background(), environment, false); code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "http://127.0.0.1:1/#bootstrap=token" {
		t.Fatalf("stdout = %q", got)
	}
}
