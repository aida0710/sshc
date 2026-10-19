package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"sshc/internal/releasecheck"
	"sshc/internal/releasewatch"
)

func newTestReleaseWatcher(t *testing.T, handler http.HandlerFunc) *releasewatch.Watcher {
	t.Helper()
	client := &http.Client{Transport: engineAPIRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		response := httptest.NewRecorder()
		handler(response, request)
		return response.Result(), nil
	})}
	checker := &releasecheck.Checker{API: "https://releases.invalid/latest", HTTP: client}
	watcher := releasewatch.New(context.Background(), func(ctx context.Context) (releasecheck.Catalog, error) {
		latest, err := checker.Latest(ctx)
		return releasecheck.Catalog{Latest: latest}, err
	})
	t.Cleanup(watcher.Stop)
	return watcher
}

func TestEngineStartupReportsOnlyANewerRelease(t *testing.T) {
	checks := 0
	watcher := newTestReleaseWatcher(t, func(w http.ResponseWriter, _ *http.Request) {
		checks++
		_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": "v0.14.0"})
	})
	var out bytes.Buffer
	reportAvailableUpdate(context.Background(), updateReportOptions{Watcher: watcher, Current: "v0.13.6", Output: &out,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if got, want := out.String(), availableUpdateNotice("v0.14.0"); got != want {
		t.Fatalf("startup update notice = %q, want %q", got, want)
	}

	out.Reset()
	reportAvailableUpdate(context.Background(), updateReportOptions{Watcher: watcher, Current: "v0.14.0", Output: &out})
	if out.Len() != 0 {
		t.Fatalf("same-version notice = %q", out.String())
	}
	if checks != 1 {
		t.Fatalf("startup repeated the retained check: %d", checks)
	}
}

func TestEngineStartupIgnoresUpdateCheckFailures(t *testing.T) {
	watcher := newTestReleaseWatcher(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	})
	var out bytes.Buffer
	reportAvailableUpdate(context.Background(), updateReportOptions{Watcher: watcher, Current: "v0.13.6", Output: &out})
	if out.Len() != 0 {
		t.Fatalf("failure notice = %q", out.String())
	}
}

func TestEngineStartupDoesNotWriteAfterCancellation(t *testing.T) {
	watcher := newTestReleaseWatcher(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": "v0.14.0"})
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	reportAvailableUpdate(ctx, updateReportOptions{Watcher: watcher, Current: "v0.13.6", Output: &out})
	if out.Len() != 0 {
		t.Fatalf("cancelled notice = %q", out.String())
	}
}
