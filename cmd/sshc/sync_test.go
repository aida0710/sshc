package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sshc/internal/api"
)

const syncOutputSecretCanary = "SYNC-OUTPUT-MUST-NOT-LEAK"

func syncStatusFixture() api.SyncStatus {
	path := "team/hosts"
	region := "ap-northeast-1"
	lastSynced := "2026-08-29T10:00:00Z"
	origin := "workstation-a"
	files := 17
	autoAt := "2026-08-29T10:05:00Z"
	autoDetail := "next check scheduled"
	objects := 4
	uploaded := int64(8192)
	return api.SyncStatus{
		Configured:    true,
		Endpoint:      "https://objects.example.test",
		Bucket:        "ssh-config",
		Path:          &path,
		Region:        &region,
		Direction:     api.SyncDirectionBoth,
		Locked:        false,
		KeyConfigured: true,
		Synced:        true,
		LastSyncedAt:  &lastSynced,
		Origin:        &origin,
		FileCount:     &files,
		Auto: api.AutoSync{
			Enabled: true, Phase: api.AutoSyncPhaseIdle, At: &autoAt, Detail: &autoDetail,
		},
		LastOperation: &api.SyncOperation{
			Kind: api.SyncOperationKindPush, CompletedAt: "2026-08-29T10:00:02Z",
			Summary: api.SnapshotSummary{
				CreatedAt: "2026-08-29T09:59:59Z", FileCount: 17,
				SourceBytes: 16384, SnapshotBytes: 4096,
			},
			ObjectCount: &objects, UploadedBytes: &uploaded,
		},
	}
}

func runSyncTestServer(t *testing.T, syncStatus int, syncBody string) (*httptest.Server, string) {
	t.Helper()
	script := &engineAPIScript{
		t: t, statusBody: validEngineStatus(), syncStatus: syncStatus, syncBody: syncBody,
	}
	server := httptest.NewServer(http.HandlerFunc(script.handler))
	stateDir := t.TempDir()
	writeTestHandoff(t, stateDir, server.URL)
	return server, stateDir
}

func TestSyncStatusHumanOutputNamesSafeOperationalState(t *testing.T) {
	body, err := json.Marshal(syncStatusFixture())
	if err != nil {
		t.Fatal(err)
	}
	server, stateDir := runSyncTestServer(t, http.StatusOK, string(body))
	defer server.Close()

	var stdout, stderr strings.Builder
	code := runSync(context.Background(), syncInvocation{Action: syncStatus}, stateDir,
		server.Client(), nil, &stdout, &stderr, nil)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	printed := stdout.String()
	for _, want := range []string{
		"configured", "yes", "vault", "unlocked", "sync key", "https://objects.example.test",
		"ssh-config", "team/hosts", "ap-northeast-1", "both", "auto", "idle",
		"2026-08-29T10:00:00Z", "workstation-a", "17", "push", "4", "8192",
		"16384", "4096", "2026-08-29T10:00:02Z",
	} {
		if !strings.Contains(printed, want) {
			t.Errorf("sync status omitted %q:\n%s", want, printed)
		}
	}
	if strings.Contains(printed, syncOutputSecretCanary) || strings.Contains(printed, "{") {
		t.Fatalf("human status leaked or printed JSON:\n%s", printed)
	}
}

func TestSyncStatusJSONUsesOneStableEnvelope(t *testing.T) {
	status := syncStatusFixture()
	body, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	server, stateDir := runSyncTestServer(t, http.StatusOK, string(body))
	defer server.Close()

	var stdout, stderr strings.Builder
	code := runSync(context.Background(), syncInvocation{Action: syncStatus, JSON: true}, stateDir,
		server.Client(), nil, &stdout, &stderr, nil)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	decoder := json.NewDecoder(strings.NewReader(stdout.String()))
	var envelope commandEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		t.Fatalf("stdout contains more than one JSON value: %q", stdout.String())
	}
	if envelope.SchemaVersion != 1 || !envelope.Success || envelope.Status == nil ||
		!envelope.Status.Configured || envelope.Failure != nil || envelope.Result != nil {
		t.Fatalf("envelope = %+v", envelope)
	}
}

func TestSyncJSONFailureIsOneStdoutObjectAndNoStderr(t *testing.T) {
	var stdout, stderr strings.Builder
	code := runSync(context.Background(), syncInvocation{Action: syncStatus, JSON: true}, t.TempDir(),
		&http.Client{}, nil, &stdout, &stderr, nil)
	if code != 1 || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	decoder := json.NewDecoder(strings.NewReader(stdout.String()))
	var envelope commandEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		t.Fatalf("stdout contains more than one JSON value: %q", stdout.String())
	}
	if envelope.SchemaVersion != 1 || envelope.Success || envelope.Status != nil ||
		envelope.Failure == nil || envelope.Failure.Kind != "engine_not_running" ||
		!envelope.Failure.Retryable {
		t.Fatalf("failure envelope = %+v", envelope)
	}
}

func TestSyncStatusHumanFailureIsActionableAndDoesNotUseStdout(t *testing.T) {
	var stdout, stderr strings.Builder
	code := runSync(context.Background(), syncInvocation{Action: syncStatus}, t.TempDir(),
		&http.Client{}, nil, &stdout, &stderr, nil)
	if code != 1 || stdout.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "sshc engine") || !strings.Contains(stderr.String(), "desktop") {
		t.Fatalf("failure is not actionable: %q", stderr.String())
	}
}

func TestRunSyncCanceledReturns130(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout, stderr strings.Builder
	code := runSync(ctx, syncInvocation{Action: syncStatus}, t.TempDir(), &http.Client{},
		nil, &stdout, &stderr, nil)
	if code != 130 {
		t.Fatalf("code=%d, want 130; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestSyncStatusFailureNeverPrintsEngineProblemMessage(t *testing.T) {
	body := `{"code":"sync_failed","message":"` + syncOutputSecretCanary + `"}`
	server, stateDir := runSyncTestServer(t, http.StatusBadGateway, body)
	defer server.Close()

	for _, asJSON := range []bool{false, true} {
		var stdout, stderr strings.Builder
		code := runSync(context.Background(), syncInvocation{Action: syncStatus, JSON: asJSON}, stateDir,
			server.Client(), nil, &stdout, &stderr, nil)
		if code != 1 {
			t.Fatalf("json=%v code=%d", asJSON, code)
		}
		if strings.Contains(stdout.String()+stderr.String(), syncOutputSecretCanary) {
			t.Fatalf("json=%v leaked engine problem: stdout=%q stderr=%q", asJSON, stdout.String(), stderr.String())
		}
	}
}
