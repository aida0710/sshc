package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sshc/internal/api"
	"sshc/internal/httpserver"
	"sshc/internal/remotesync"
	"sshc/internal/session"
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

func TestSyncStatusHumanOutputEscapesTerminalControls(t *testing.T) {
	status := syncStatusFixture()
	origin := "remote\x1b]52;c;payload\a\nnext"
	status.Origin = &origin
	body, err := json.Marshal(status)
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
	if strings.ContainsAny(printed, "\x1b\a") || !strings.Contains(printed, `\u001B`) ||
		!strings.Contains(printed, `\u0007`) || !strings.Contains(printed, `\u000A`) {
		t.Fatalf("terminal controls were not escaped: %q", printed)
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

func TestSyncUnknownMutationOutcomeIsNeverReportedAsRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		exit int
	}{
		{
			name: "transport failure",
			err: engineProblem{
				Code: "transport_error", Retryable: true, OutcomeUnknown: true,
			},
			exit: 1,
		},
		{
			name: "invalid successful response",
			err: engineProblem{
				Status: http.StatusOK, Code: "invalid_response", OutcomeUnknown: true,
			},
			exit: 1,
		},
		{
			name: "canceled mutation",
			err: engineProblem{
				Code: "transport_error", Retryable: false, OutcomeUnknown: true,
				cause: context.Canceled,
			},
			exit: 130,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, asJSON := range []bool{false, true} {
				var stdout, stderr strings.Builder
				if code := finishSyncFailure(asJSON, test.err, &stdout, &stderr); code != test.exit {
					t.Fatalf("json=%v code=%d, want %d", asJSON, code, test.exit)
				}
				if asJSON {
					if stderr.Len() != 0 {
						t.Fatalf("JSON failure wrote stderr %q", stderr.String())
					}
					var envelope commandEnvelope
					if err := json.Unmarshal([]byte(stdout.String()), &envelope); err != nil {
						t.Fatal(err)
					}
					if envelope.Failure == nil || envelope.Failure.Kind != "outcome_unknown" ||
						envelope.Failure.Retryable {
						t.Fatalf("failure = %+v", envelope.Failure)
					}
					continue
				}
				if stdout.Len() != 0 || !strings.Contains(stderr.String(), "outcome is unknown") ||
					!strings.Contains(stderr.String(), "do not rerun") || strings.Contains(stderr.String(), "try again") {
					t.Fatalf("human failure stdout=%q stderr=%q", stdout.String(), stderr.String())
				}
			}
		})
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

type syncCommandHarness struct {
	t       *testing.T
	base    *engineAPIScript
	handle  func(http.ResponseWriter, *http.Request)
	paths   []string
	bodies  [][]byte
	methods []string
}

func (harness *syncCommandHarness) handler(response http.ResponseWriter, request *http.Request) {
	switch request.URL.Path {
	case httpserver.StatusPath, httpserver.CLISessionPath:
		harness.base.handler(response, request)
		return
	}
	harness.paths = append(harness.paths, request.URL.Path)
	harness.methods = append(harness.methods, request.Method)
	body, err := io.ReadAll(request.Body)
	if err != nil {
		harness.t.Error(err)
	}
	harness.bodies = append(harness.bodies, body)
	request.Body = io.NopCloser(bytes.NewReader(body))
	if cookie, err := request.Cookie(httpserver.SessionCookie); err != nil || cookie.Value != engineAPISessionCanary {
		harness.t.Errorf("operation cookie = %#v, %v", cookie, err)
	}
	if request.Header.Get(httpserver.CSRFHeader) != engineAPICSRFCanary ||
		request.Header.Get("Sec-Fetch-Site") != "same-origin" {
		harness.t.Errorf("operation authentication headers are incomplete")
	}
	if request.Method != http.MethodGet && request.Header.Get("Origin") != "http://"+request.Host {
		harness.t.Errorf("operation origin = %q", request.Header.Get("Origin"))
	}
	harness.handle(response, request)
}

func newSyncCommandHarness(
	t *testing.T, handle func(http.ResponseWriter, *http.Request),
) (*syncCommandHarness, *httptest.Server, string) {
	t.Helper()
	harness := &syncCommandHarness{
		t: t,
		base: &engineAPIScript{
			t: t, statusBody: validEngineStatus(), syncBody: `{"configured":true}`,
		},
		handle: handle,
	}
	server := httptest.NewServer(http.HandlerFunc(harness.handler))
	stateDir := t.TempDir()
	writeTestHandoff(t, stateDir, server.URL)
	return harness, server, stateDir
}

func pushResponseFixture() api.PushResponse {
	return api.PushResponse{
		Status: syncStatusFixture(),
		Result: api.PushResult{
			CompletedAt: "2026-08-29T11:00:00Z", ObjectCount: 3, UploadedBytes: 12288,
			Summary: api.SnapshotSummary{
				CreatedAt: "2026-08-29T10:59:59Z", FileCount: 17,
				SourceBytes: 16384, SnapshotBytes: 4096,
			},
		},
	}
}

func TestSyncPushUsesEngineDraftThenPushesOnce(t *testing.T) {
	for _, asJSON := range []bool{false, true} {
		t.Run(map[bool]string{false: "human", true: "json"}[asJSON], func(t *testing.T) {
			var pushBody api.SyncPushRequest
			harness, server, stateDir := newSyncCommandHarness(t, func(response http.ResponseWriter, request *http.Request) {
				response.Header().Set("Content-Type", "application/json")
				switch {
				case request.Method == http.MethodGet && request.URL.Path == "/api/v1/sync/push":
					_ = json.NewEncoder(response).Encode(api.SyncPushDraft{
						Message: "engine-generated draft", Added: 1, Modified: 2, Removed: 0,
					})
				case request.Method == http.MethodPost && request.URL.Path == "/api/v1/sync/push":
					if err := json.NewDecoder(request.Body).Decode(&pushBody); err != nil {
						t.Error(err)
					}
					_ = json.NewEncoder(response).Encode(pushResponseFixture())
				default:
					response.WriteHeader(http.StatusNotFound)
				}
			})
			defer server.Close()
			var stdout, stderr strings.Builder
			code := runSync(context.Background(), syncInvocation{Action: syncPush, JSON: asJSON}, stateDir,
				server.Client(), nil, &stdout, &stderr, nil)
			if code != 0 || stderr.Len() != 0 || pushBody.Message != "engine-generated draft" {
				t.Fatalf("code=%d request=%+v stdout=%q stderr=%q", code, pushBody, stdout.String(), stderr.String())
			}
			if strings.Join(harness.methods, ",") != "GET,POST" ||
				strings.Join(harness.paths, ",") != "/api/v1/sync/push,/api/v1/sync/push" {
				t.Fatalf("operations = %v %v", harness.methods, harness.paths)
			}
			for _, want := range []string{"17", "3", "12288", "16384", "4096"} {
				if !strings.Contains(stdout.String(), want) {
					t.Errorf("output omitted %q: %s", want, stdout.String())
				}
			}
			if asJSON {
				var envelope commandEnvelope
				if err := json.Unmarshal([]byte(stdout.String()), &envelope); err != nil || !envelope.Success || envelope.Result == nil {
					t.Fatalf("JSON result = %+v, %v", envelope, err)
				}
			}
		})
	}
}

func TestSyncPushRemoteMoveIsNotRetried(t *testing.T) {
	harness, server, stateDir := newSyncCommandHarness(t, func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodGet {
			_ = json.NewEncoder(response).Encode(api.SyncPushDraft{Message: "one draft"})
			return
		}
		response.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(response, `{"code":"sync_remote_moved","message":"do not retry"}`)
	})
	defer server.Close()
	var stdout, stderr strings.Builder
	code := runSync(context.Background(), syncInvocation{Action: syncPush, JSON: true}, stateDir,
		server.Client(), nil, &stdout, &stderr, nil)
	if code != 1 || len(harness.paths) != 2 || stderr.Len() != 0 {
		t.Fatalf("code=%d paths=%v stdout=%q stderr=%q", code, harness.paths, stdout.String(), stderr.String())
	}
	var envelope commandEnvelope
	if err := json.Unmarshal([]byte(stdout.String()), &envelope); err != nil || envelope.Failure == nil ||
		envelope.Failure.Kind != "sync_remote_moved" || !envelope.Failure.Retryable {
		t.Fatalf("failure = %+v, %v", envelope, err)
	}
}

func TestSyncForcePushUsesDraftThenOneExactActionToken(t *testing.T) {
	var actionBody api.IssueActionRequest
	var forceBody api.SyncPushRequest
	harness, server, stateDir := newSyncCommandHarness(t, func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/sync/push":
			_ = json.NewEncoder(response).Encode(api.SyncPushDraft{Message: "force engine draft", Modified: 1})
		case "/api/v1/actions":
			if err := json.NewDecoder(request.Body).Decode(&actionBody); err != nil {
				t.Error(err)
			}
			response.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(response).Encode(api.IssueActionResponse{Token: "one-action-token", ExpiresAt: "soon"})
		case "/api/v1/sync/force-push":
			if got := request.Header.Get(httpserver.ActionHeader); got != "one-action-token" {
				t.Errorf("action token = %q", got)
			}
			if err := json.NewDecoder(request.Body).Decode(&forceBody); err != nil {
				t.Error(err)
			}
			_ = json.NewEncoder(response).Encode(pushResponseFixture())
		default:
			response.WriteHeader(http.StatusNotFound)
		}
	})
	defer server.Close()
	var stdout, stderr strings.Builder
	code := runSync(context.Background(), syncInvocation{Action: syncPush, Force: true}, stateDir,
		server.Client(), nil, &stdout, &stderr, nil)
	if code != 0 || stderr.Len() != 0 || actionBody.Kind != session.ActionSyncForcePush ||
		actionBody.Target != remotesync.ForcePushTarget || forceBody.Message != "force engine draft" {
		t.Fatalf("code=%d action=%+v force=%+v stdout=%q stderr=%q",
			code, actionBody, forceBody, stdout.String(), stderr.String())
	}
	if strings.Join(harness.paths, ",") != "/api/v1/sync/push,/api/v1/actions,/api/v1/sync/force-push" {
		t.Fatalf("force sequence = %v", harness.paths)
	}
}

func TestSyncForcePushRemoteMoveDoesNotAcquireAnotherTokenOrRetry(t *testing.T) {
	harness, server, stateDir := newSyncCommandHarness(t, func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/sync/push":
			_ = json.NewEncoder(response).Encode(api.SyncPushDraft{Message: "force once"})
		case "/api/v1/actions":
			response.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(response).Encode(api.IssueActionResponse{Token: "single-token", ExpiresAt: "soon"})
		case "/api/v1/sync/force-push":
			response.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(response, `{"code":"sync_remote_moved","message":"moved"}`)
		}
	})
	defer server.Close()
	var stdout, stderr strings.Builder
	code := runSync(context.Background(), syncInvocation{Action: syncPush, Force: true}, stateDir,
		server.Client(), nil, &stdout, &stderr, nil)
	if code != 1 || len(harness.paths) != 3 {
		t.Fatalf("code=%d sequence=%v stdout=%q stderr=%q", code, harness.paths, stdout.String(), stderr.String())
	}
}

func pullResponseFixture() api.PullResponse {
	return api.PullResponse{
		Applied: false, CompletedAt: "2026-08-29T12:00:00Z", DownloadedBytes: 4096,
		RemoteETag: "preview-etag", RemoteRevision: strings.Repeat("a", 64),
		Conflicts: []api.SyncConflict{}, Written: []string{"config"}, Removed: []string{},
		Summary: api.SnapshotSummary{
			CreatedAt: "2026-08-29T11:59:59Z", FileCount: 3, SourceBytes: 8192, SnapshotBytes: 2048,
		},
	}
}

func TestSyncPullSafeWritesPreviewThenApplyExactIdentity(t *testing.T) {
	var requests []api.PullRequest
	harness, server, stateDir := newSyncCommandHarness(t, func(response http.ResponseWriter, request *http.Request) {
		var body api.PullRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		requests = append(requests, body)
		result := pullResponseFixture()
		if len(requests) == 2 {
			result.Applied = true
			result.CompletedAt = "2026-08-29T12:00:03Z"
		}
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(result)
	})
	defer server.Close()
	var stdout, stderr strings.Builder
	code := runSync(context.Background(), syncInvocation{Action: syncPull, JSON: true}, stateDir,
		server.Client(), nil, &stdout, &stderr, nil)
	if code != 0 || stderr.Len() != 0 || len(requests) != 2 || len(harness.paths) != 2 {
		t.Fatalf("code=%d requests=%+v stdout=%q stderr=%q", code, requests, stdout.String(), stderr.String())
	}
	if requests[0].Apply == nil || *requests[0].Apply || requests[0].Resolve != nil {
		t.Fatalf("preview request = %+v", requests[0])
	}
	if requests[1].Apply == nil || !*requests[1].Apply || requests[1].Resolve != nil ||
		requests[1].ExpectedETag == nil || *requests[1].ExpectedETag != "preview-etag" ||
		requests[1].ExpectedRevision == nil || *requests[1].ExpectedRevision != strings.Repeat("a", 64) {
		t.Fatalf("apply request = %+v", requests[1])
	}
	var envelope commandEnvelope
	if err := json.Unmarshal([]byte(stdout.String()), &envelope); err != nil || !envelope.Success || envelope.Result == nil {
		t.Fatalf("pull envelope = %+v, %v", envelope, err)
	}
}

func TestSyncPullNormalRefusesConflictOrRemovalBeforeApply(t *testing.T) {
	for _, test := range []struct {
		name      string
		conflicts []api.SyncConflict
		removed   []string
	}{
		{name: "conflict", conflicts: []api.SyncConflict{{Path: "config", ChangedHere: true, ChangedThere: true}}, removed: []string{}},
		{name: "removal", conflicts: []api.SyncConflict{}, removed: []string{"old.conf"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			harness, server, stateDir := newSyncCommandHarness(t, func(response http.ResponseWriter, request *http.Request) {
				calls++
				result := pullResponseFixture()
				result.Conflicts = test.conflicts
				result.Removed = test.removed
				response.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(response).Encode(result)
			})
			defer server.Close()
			var stdout, stderr strings.Builder
			code := runSync(context.Background(), syncInvocation{Action: syncPull, JSON: true}, stateDir,
				server.Client(), nil, &stdout, &stderr, nil)
			if code != 1 || calls != 1 || len(harness.paths) != 1 || stderr.Len() != 0 {
				t.Fatalf("code=%d calls=%d paths=%v stdout=%q stderr=%q",
					code, calls, harness.paths, stdout.String(), stderr.String())
			}
			var envelope commandEnvelope
			if err := json.Unmarshal([]byte(stdout.String()), &envelope); err != nil || envelope.Failure == nil ||
				envelope.Failure.Kind != "sync_pull_requires_force" {
				t.Fatalf("failure = %+v, %v", envelope, err)
			}
		})
	}
}

func TestSyncPullForceUsesRemoteResolutionForPreviewAndExactApply(t *testing.T) {
	var requests []api.PullRequest
	_, server, stateDir := newSyncCommandHarness(t, func(response http.ResponseWriter, request *http.Request) {
		var body api.PullRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		requests = append(requests, body)
		result := pullResponseFixture()
		result.Conflicts = []api.SyncConflict{{Path: "config", ChangedHere: true, ChangedThere: true}}
		result.Removed = []string{"old.conf"}
		if len(requests) == 2 {
			result.Applied = true
		}
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(result)
	})
	defer server.Close()
	var stdout, stderr strings.Builder
	code := runSync(context.Background(), syncInvocation{Action: syncPull, Force: true}, stateDir,
		server.Client(), nil, &stdout, &stderr, nil)
	if code != 0 || len(requests) != 2 || stderr.Len() != 0 {
		t.Fatalf("code=%d requests=%+v stdout=%q stderr=%q", code, requests, stdout.String(), stderr.String())
	}
	for index, request := range requests {
		if request.Resolve == nil || *request.Resolve != api.Remote {
			t.Fatalf("request %d resolve = %+v", index, request.Resolve)
		}
		if request.AcceptRemoteHead == nil || !*request.AcceptRemoteHead {
			t.Fatalf("request %d accept remote head = %+v", index, request.AcceptRemoteHead)
		}
	}
	if requests[1].ExpectedETag == nil || *requests[1].ExpectedETag != "preview-etag" ||
		requests[1].ExpectedRevision == nil || *requests[1].ExpectedRevision != strings.Repeat("a", 64) {
		t.Fatalf("force apply lost preview identity: %+v", requests[1])
	}
}

func TestSyncPullStaleApplyDoesNotRepreviewOrRetry(t *testing.T) {
	calls := 0
	harness, server, stateDir := newSyncCommandHarness(t, func(response http.ResponseWriter, request *http.Request) {
		calls++
		response.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			_ = json.NewEncoder(response).Encode(pullResponseFixture())
			return
		}
		response.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(response, `{"code":"preview_stale","message":"stale"}`)
	})
	defer server.Close()
	var stdout, stderr strings.Builder
	code := runSync(context.Background(), syncInvocation{Action: syncPull, Force: true, JSON: true}, stateDir,
		server.Client(), nil, &stdout, &stderr, nil)
	if code != 1 || calls != 2 || len(harness.paths) != 2 || stderr.Len() != 0 {
		t.Fatalf("code=%d calls=%d paths=%v stdout=%q stderr=%q", code, calls, harness.paths, stdout.String(), stderr.String())
	}
	var envelope commandEnvelope
	if err := json.Unmarshal([]byte(stdout.String()), &envelope); err != nil || envelope.Failure == nil ||
		envelope.Failure.Kind != "preview_stale" || !envelope.Failure.Retryable {
		t.Fatalf("failure = %+v, %v", envelope, err)
	}
}

func TestSyncPullNoChangesStillAcknowledgesTheRemoteGeneration(t *testing.T) {
	calls := 0
	_, server, stateDir := newSyncCommandHarness(t, func(response http.ResponseWriter, request *http.Request) {
		calls++
		result := pullResponseFixture()
		result.Written = []string{}
		result.Removed = []string{}
		result.Applied = calls == 2
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(result)
	})
	defer server.Close()
	var stdout, stderr strings.Builder
	code := runSync(context.Background(), syncInvocation{Action: syncPull}, stateDir,
		server.Client(), nil, &stdout, &stderr, nil)
	if code != 0 || calls != 2 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "applied") {
		t.Fatalf("code=%d calls=%d stdout=%q stderr=%q", code, calls, stdout.String(), stderr.String())
	}
}

func TestSyncNowCallsExistingEngineOperationOnce(t *testing.T) {
	harness, server, stateDir := newSyncCommandHarness(t, func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(syncStatusFixture())
	})
	defer server.Close()
	var stdout, stderr strings.Builder
	code := runSync(context.Background(), syncInvocation{Action: syncNow}, stateDir,
		server.Client(), nil, &stdout, &stderr, nil)
	if code != 0 || stderr.Len() != 0 || len(harness.paths) != 1 ||
		harness.paths[0] != "/api/v1/sync/now" || harness.methods[0] != http.MethodPost ||
		string(harness.bodies[0]) != "{}" {
		t.Fatalf("code=%d methods=%v paths=%v bodies=%q stdout=%q stderr=%q",
			code, harness.methods, harness.paths, harness.bodies, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "https://objects.example.test") || !strings.Contains(stdout.String(), "idle") {
		t.Fatalf("now did not render returned status: %q", stdout.String())
	}
}

func TestSyncAutoSendsExactPersistentSettingAndReturnsJSONStatus(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(map[bool]string{true: "on", false: "off"}[enabled], func(t *testing.T) {
			harness, server, stateDir := newSyncCommandHarness(t, func(response http.ResponseWriter, request *http.Request) {
				status := syncStatusFixture()
				status.Auto.Enabled = enabled
				response.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(response).Encode(status)
			})
			defer server.Close()
			var stdout, stderr strings.Builder
			code := runSync(context.Background(), syncInvocation{Action: syncAuto, Enabled: enabled, JSON: true}, stateDir,
				server.Client(), nil, &stdout, &stderr, nil)
			wantBody := `{"enabled":false}`
			if enabled {
				wantBody = `{"enabled":true}`
			}
			if code != 0 || stderr.Len() != 0 || len(harness.paths) != 1 ||
				harness.paths[0] != "/api/v1/sync/auto" || harness.methods[0] != http.MethodPut ||
				string(harness.bodies[0]) != wantBody {
				t.Fatalf("code=%d methods=%v paths=%v bodies=%q stdout=%q stderr=%q",
					code, harness.methods, harness.paths, harness.bodies, stdout.String(), stderr.String())
			}
			var envelope commandEnvelope
			if err := json.Unmarshal([]byte(stdout.String()), &envelope); err != nil ||
				!envelope.Success || envelope.Result == nil || envelope.Failure != nil {
				t.Fatalf("auto envelope = %+v, %v", envelope, err)
			}
		})
	}
}
