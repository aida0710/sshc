package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sshc/internal/selfupdate"
)

func updateCheckerServer(t *testing.T, handler http.HandlerFunc) *selfupdate.Checker {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &selfupdate.Checker{API: server.URL, HTTP: server.Client()}
}

func TestEngineStartupReportsOnlyANewerRelease(t *testing.T) {
	checker := updateCheckerServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": "v0.14.0"})
	})
	var out bytes.Buffer
	reportAvailableUpdate(context.Background(), checker, "v0.13.6", &out,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if got := out.String(); !strings.Contains(got, "v0.14.0 is available") || !strings.Contains(got, "sshc update") {
		t.Fatalf("startup update notice = %q", got)
	}

	out.Reset()
	reportAvailableUpdate(context.Background(), checker, "v0.14.0", &out, nil)
	if out.Len() != 0 {
		t.Fatalf("same-version notice = %q", out.String())
	}
}

func TestEngineStartupIgnoresUpdateCheckFailures(t *testing.T) {
	checker := updateCheckerServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	})
	var out bytes.Buffer
	reportAvailableUpdate(context.Background(), checker, "v0.13.6", &out, nil)
	if out.Len() != 0 {
		t.Fatalf("failure notice = %q", out.String())
	}
}

func TestEngineStartupDoesNotWriteAfterCancellation(t *testing.T) {
	checker := updateCheckerServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": "v0.14.0"})
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	reportAvailableUpdate(ctx, checker, "v0.13.6", &out, nil)
	if out.Len() != 0 {
		t.Fatalf("cancelled notice = %q", out.String())
	}
}
