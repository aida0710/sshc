package httpserver

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/objectstore"
	"sshc/internal/remotesync"
	"sshc/internal/secret"
	"sshc/internal/session"
	"sshc/internal/storage"
)

func syncEngine(t *testing.T) (*echo.Echo, *remotesync.Service) {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	service := remotesync.NewService(workspace,
		storage.NewManager(workspace, time.Now, rand.Reader),
		func() string { return "2026-08-05T00:00:00Z" },
		func() (string, error) { return "origin-test", nil },
	)

	secrets := secret.NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader), time.Now)
	if err := secrets.Initialise(syncTestPassphrase); err != nil {
		t.Fatal(err)
	}
	// 暗号化する鍵は保管庫から来る。押したユーザーが打つものではない。
	if err := secrets.SetSyncKey(measuredSyncKey); err != nil {
		t.Fatal(err)
	}
	// vault は復号済み文書として同期し、受信側で再暗号化する。
	service.OpenVault = secrets.TravelDocument
	service.SealVault = secrets.AdoptTravelDocument
	service.VaultAdopted = secrets.Reload
	engine := echo.New()
	registerSyncRoutes(engine, SyncHandlers{Service: service, Secrets: secrets, Reach: reachable})
	return engine, service
}

// measuredSyncKey は、この一群のテストがリモートを暗号化する鍵である。
const measuredSyncKey = "AB12-CD34-EF56-GH78-JK90-MN12"

const syncTestPassphrase = "a master password for sync"

// reachable は bucket の代わりを務める。「この bucket は応答するか」
// という問いは remotesync のものであり、そちらでは実物の HTTP
// サーバーに対してテストされる。ここではネットワークに問うてはならない。
func reachable(context.Context, *objectstore.Client, string) error { return nil }

func syncEngineWithVault(t *testing.T) (*echo.Echo, *remotesync.Service, *secret.Service) {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	manager := storage.NewManager(workspace, time.Now, rand.Reader)
	service := remotesync.NewService(workspace, manager,
		func() string { return "2026-08-05T00:00:00Z" },
		func() (string, error) { return "origin-test", nil },
	)
	secrets := secret.NewService(workspace, manager, time.Now)

	engine := echo.New()
	registerSyncRoutes(engine, SyncHandlers{Service: service, Secrets: secrets, Reach: reachable})
	return engine, service, secrets
}

func sendSync(t *testing.T, engine *echo.Echo, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	return send(t, engine, method, path, body, nil)
}

func settings(direction string) string {
	body := `{"endpoint":"https://example.invalid","bucket":"sshc",` +
		`"accessKeyId":"AKID","secretAccessKey":"secret"`
	if direction != "" {
		body += `,"direction":"` + direction + `"`
	}
	return body + "}"
}

type measuredSyncObject struct {
	body []byte
	etag string
}

// measuredSyncBucket is deliberately an HTTP server, not a stubbed Service.
// The API measurements must describe the bytes that crossed the object-store
// boundary, so this test observes that boundary directly.
type measuredSyncBucket struct {
	mu         sync.Mutex
	objects    map[string]measuredSyncObject
	putBytes   []int
	generation int
}

func (b *measuredSyncBucket) handler(w http.ResponseWriter, request *http.Request) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.objects == nil {
		b.objects = map[string]measuredSyncObject{}
	}
	key := strings.TrimPrefix(strings.TrimPrefix(request.URL.Path, "/"), "sshc/")
	stored, present := b.objects[key]
	switch request.Method {
	case http.MethodGet, http.MethodHead:
		if !present {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("ETag", stored.etag)
		if request.Method == http.MethodGet {
			_, _ = w.Write(stored.body)
		}
	case http.MethodPut:
		if request.Header.Get("If-None-Match") == "*" && present {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		if expected := request.Header.Get("If-Match"); expected != "" && expected != stored.etag {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		b.generation++
		etag := `"generation-` + string(rune('0'+b.generation)) + `"`
		b.objects[key] = measuredSyncObject{body: body, etag: etag}
		b.putBytes = append(b.putBytes, len(body))
		w.Header().Set("ETag", etag)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (b *measuredSyncBucket) liveBytes() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.objects[remotesync.ObjectName].body)
}

func (b *measuredSyncBucket) liveETag() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.objects[remotesync.ObjectName].etag
}

func (b *measuredSyncBucket) uploadedBytes() (int, int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	total := 0
	for _, size := range b.putBytes {
		total += size
	}
	return len(b.putBytes), total
}

type measuredSyncInstallation struct {
	engine      *echo.Echo
	service     *remotesync.Service
	secrets     *secret.Service
	config      remotesync.Config
	credentials objectstore.Credentials
	client      *objectstore.Client
}

func newMeasuredSyncInstallation(t *testing.T, bucket *measuredSyncBucket, files map[string]string) measuredSyncInstallation {
	t.Helper()
	home := t.TempDir()
	root := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, contents := range files {
		absolute := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	service := remotesync.NewService(
		workspace,
		storage.NewManager(workspace, time.Now, rand.Reader),
		func() string { return "2026-08-12T01:02:03Z" },
		func() (string, error) { return "origin-api-test", nil },
	)
	server := httptest.NewTLSServer(http.HandlerFunc(bucket.handler))
	t.Cleanup(server.Close)
	credentials := objectstore.Credentials{AccessKeyID: "AKID", SecretAccessKey: "secret"}
	config := remotesync.Config{Endpoint: server.URL, Bucket: "sshc", Region: "auto", Direction: remotesync.DirectionBoth}
	client := &objectstore.Client{
		HTTP: server.Client(), Endpoint: server.URL, Bucket: "sshc", Region: "auto", Creds: credentials,
	}
	if err := service.Configure(config, credentials, client); err != nil {
		t.Fatal(err)
	}
	secrets := secret.NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader), time.Now)
	if err := secrets.Initialise(syncTestPassphrase); err != nil {
		t.Fatal(err)
	}
	if err := secrets.SetSyncKey(measuredSyncKey); err != nil {
		t.Fatal(err)
	}
	// vault は復号済み文書として同期し、受信側で再暗号化する。
	service.OpenVault = secrets.TravelDocument
	service.SealVault = secrets.AdoptTravelDocument
	service.VaultAdopted = secrets.Reload
	engine := echo.New()
	registerSyncRoutes(engine, SyncHandlers{Service: service, Secrets: secrets, Reach: reachable})
	return measuredSyncInstallation{
		engine: engine, service: service, secrets: secrets,
		config: config, credentials: credentials, client: client,
	}
}

func measuredSyncEngine(t *testing.T, bucket *measuredSyncBucket, files map[string]string) (*echo.Echo, *remotesync.Service, *secret.Service) {
	installation := newMeasuredSyncInstallation(t, bucket, files)
	return installation.engine, installation.service, installation.secrets
}

func TestPushResponseReportsTheMeasuredTransferAndLastSuccessfulOperation(t *testing.T) {
	bucket := &measuredSyncBucket{}
	engine, service, _ := measuredSyncEngine(t, bucket, map[string]string{"config": "Host edge\n"})

	// 数を直接書かない。保管庫のファイル自身もスナップショットに載る
	// （Collect が sshc/secrets を指定している）ので、書いた数は「この
	// テストが用意したファイルの数」ではない。集めたものと突き合わせる。
	collected, contents, err := service.Collect()
	if err != nil {
		t.Fatal(err)
	}
	wantSource := int64(0)
	for _, body := range contents {
		wantSource += int64(len(body))
	}

	draft := sendSync(t, engine, http.MethodGet, "/api/v1/sync/push", "")
	if draft.Code != http.StatusOK || !strings.Contains(draft.Body.String(), `"message"`) {
		t.Fatalf("push draft = %d: %s", draft.Code, draft.Body.String())
	}
	response := sendSync(t, engine, http.MethodPost, "/api/v1/sync/push", `{"message":"Set up edge hosts"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("push = %d: %s", response.Code, response.Body.String())
	}
	var body struct {
		Status struct {
			LastOperation *remotesync.SyncOperation `json:"lastOperation"`
		} `json:"status"`
		Result remotesync.PushResult `json:"result"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	putCount, uploadedBytes := bucket.uploadedBytes()
	if body.Result.Summary.FileCount != len(collected.Files) || body.Result.Summary.SourceBytes != wantSource {
		t.Errorf("source summary = %+v, collected %d files / %d bytes",
			body.Result.Summary, len(collected.Files), wantSource)
	}
	if body.Result.Summary.SnapshotBytes != int64(bucket.liveBytes()) {
		t.Errorf("snapshot bytes = %d, live object = %d", body.Result.Summary.SnapshotBytes, bucket.liveBytes())
	}
	if body.Result.ObjectCount != putCount || body.Result.UploadedBytes != int64(uploadedBytes) {
		t.Errorf("transfer = %+v, HTTP observed %d objects / %d bytes", body.Result, putCount, uploadedBytes)
	}
	if body.Result.CompletedAt == "" || body.Result.Summary.CreatedAt == "" {
		t.Errorf("timestamps are missing: %+v", body.Result)
	}
	if body.Status.LastOperation == nil || body.Status.LastOperation.Kind != remotesync.OperationPush {
		t.Errorf("push status did not carry its last successful operation: %+v", body.Status.LastOperation)
	}

	reloaded := sendSync(t, engine, http.MethodGet, "/api/v1/sync", "")
	var reloadedBody struct {
		LastOperation *remotesync.SyncOperation `json:"lastOperation"`
	}
	if err := json.Unmarshal(reloaded.Body.Bytes(), &reloadedBody); err != nil {
		t.Fatal(err)
	}
	if reloadedBody.LastOperation == nil || reloadedBody.LastOperation.Kind != remotesync.OperationPush {
		t.Errorf("reloaded status lost the persisted operation: %s", reloaded.Body.String())
	}

	unchanged := sendSync(t, engine, http.MethodPost, "/api/v1/sync/push", `{"message":"Duplicate edge hosts"}`)
	if unchanged.Code != http.StatusConflict || !strings.Contains(unchanged.Body.String(), `"code":"sync_nothing_to_push"`) {
		t.Fatalf("unchanged push = %d: %s", unchanged.Code, unchanged.Body.String())
	}
	afterCount, _ := bucket.uploadedBytes()
	if afterCount != putCount {
		t.Fatalf("unchanged push wrote %d more objects", afterCount-putCount)
	}
}

func TestSyncExclusionsExposeDefaultsAndSaveSharedRules(t *testing.T) {
	bucket := &measuredSyncBucket{}
	installation := newMeasuredSyncInstallation(t, bucket, map[string]string{
		"config": "Host edge\n", "cache/session.tmp": "temporary", "keys/work/id_ed25519": "private",
	})

	defaults := sendSync(t, installation.engine, http.MethodGet, "/api/v1/sync/exclusions", "")
	if defaults.Code != http.StatusOK {
		t.Fatalf("get exclusions = %d: %s", defaults.Code, defaults.Body.String())
	}
	var defaultView api.SyncExclusions
	if err := json.Unmarshal(defaults.Body.Bytes(), &defaultView); err != nil {
		t.Fatal(err)
	}
	if !defaultView.UsingDefaults || defaultView.Document != remotesync.DefaultIgnoreDocument {
		t.Fatalf("default exclusions = %+v", defaultView)
	}
	if len(defaultView.Candidates) != 3 {
		t.Fatalf("candidates = %+v", defaultView.Candidates)
	}

	saved := sendSync(t, installation.engine, http.MethodPut, "/api/v1/sync/exclusions", `{"document":"keys/**\n"}`)
	if saved.Code != http.StatusOK {
		t.Fatalf("save exclusions = %d: %s", saved.Code, saved.Body.String())
	}
	var savedView api.SyncExclusions
	if err := json.Unmarshal(saved.Body.Bytes(), &savedView); err != nil {
		t.Fatal(err)
	}
	if savedView.UsingDefaults || savedView.Document != "keys/**\n" {
		t.Fatalf("saved exclusions = %+v", savedView)
	}
	manifest, _, err := installation.service.Collect()
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	for _, entry := range manifest.Files {
		paths[entry.Path] = true
	}
	if paths["keys/work/id_ed25519"] || !paths[remotesync.IgnorePath] || !paths["cache/session.tmp"] {
		t.Fatalf("collected paths = %v", paths)
	}

	invalid := sendSync(t, installation.engine, http.MethodPut, "/api/v1/sync/exclusions", `{"document":"broken[\n"}`)
	if invalid.Code != http.StatusBadRequest || !strings.Contains(invalid.Body.String(), `"code":"sync_ignore_invalid"`) {
		t.Fatalf("invalid exclusions = %d: %s", invalid.Code, invalid.Body.String())
	}
}

func TestPushRejectsRequestsFromOlderClients(t *testing.T) {
	engine, _ := syncEngine(t)
	for name, body := range map[string]string{
		"no body":               "",
		"empty object":          `{}`,
		"deprecated passphrase": `{"passphrase":"old-client-value"}`,
	} {
		t.Run(name, func(t *testing.T) {
			response := send(t, engine, http.MethodPost, "/api/v1/sync/push", body, nil)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("push = %d, want 400: %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestForcePushRequiresAOneTimeConfirmationForTheCurrentRemoteGeneration(t *testing.T) {
	bucket := &measuredSyncBucket{}
	_, service, secrets := measuredSyncEngine(t, bucket, map[string]string{"config": "Host replacement\n"})
	if _, err := service.Push(context.Background(), measuredSyncKey, ""); err != nil {
		t.Fatalf("initial Push = %v", err)
	}

	manager, bootstrap, err := session.NewManager(bytes.NewReader(bytes.Repeat([]byte{0x61}, 4096)))
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := manager.Bootstrap(bootstrap)
	if err != nil {
		t.Fatal(err)
	}
	engine := echo.New()
	engine.Use((Security{ExpectedHost: keyTestHost, ExpectedOrigin: "http://" + keyTestHost, Sessions: manager, Unlocked: alwaysUnlocked}).Middleware)
	registry := actionRegistry{}
	addSyncActions(registry, service)
	actions := ActionHandlers{Sessions: manager, Kinds: registry}
	registerActionRoutes(engine, actions)
	registerSyncRoutes(engine, SyncHandlers{Service: service, Secrets: secrets, Reach: reachable, Actions: actions})

	before := bucket.liveETag()
	requestBody := []byte(`{"message":"Replace remote workspace"}`)
	without := sendKeyRequest(t, engine, credentials, http.MethodPost, "/api/v1/sync/force-push", requestBody, "")
	if without.Code != http.StatusForbidden || bucket.liveETag() != before {
		t.Fatalf("force push without confirmation = %d, ETag %q -> %q", without.Code, before, bucket.liveETag())
	}

	token := issueToken(t, engine, credentials, session.ActionSyncForcePush, remotesync.ForcePushTarget)
	forced := sendKeyRequest(t, engine, credentials, http.MethodPost, "/api/v1/sync/force-push", requestBody, token)
	if forced.Code != http.StatusOK || bucket.liveETag() == before {
		t.Fatalf("confirmed force push = %d: %s", forced.Code, forced.Body.String())
	}
	replayed := sendKeyRequest(t, engine, credentials, http.MethodPost, "/api/v1/sync/force-push", requestBody, token)
	if replayed.Code != http.StatusForbidden {
		t.Fatalf("replayed confirmation = %d, want 403: %s", replayed.Code, replayed.Body.String())
	}
}

func TestPullResponseReportsEachDownloadAndTheAppliedOperation(t *testing.T) {
	bucket := &measuredSyncBucket{}
	_, producer, _ := measuredSyncEngine(t, bucket, map[string]string{"config": "Host edge\n"})
	// producer も consumer も、同じ鍵で暗号化して開く。それが「端末をまたいで
	// 共有される鍵」の意味である。
	if _, err := producer.Push(context.Background(), measuredSyncKey, ""); err != nil {
		t.Fatal(err)
	}
	engine, _, _ := measuredSyncEngine(t, bucket, map[string]string{})

	preview := sendSync(t, engine, http.MethodPost, "/api/v1/sync/pull", `{"apply":false}`)
	if preview.Code != http.StatusOK {
		t.Fatalf("preview = %d: %s", preview.Code, preview.Body.String())
	}
	var previewBody struct {
		Applied         bool                       `json:"applied"`
		Summary         remotesync.SnapshotSummary `json:"summary"`
		DownloadedBytes int64                      `json:"downloadedBytes"`
		CompletedAt     string                     `json:"completedAt"`
		Written         []string                   `json:"written"`
		RemoteETag      string                     `json:"remoteETag"`
		RemoteRevision  string                     `json:"remoteRevision"`
	}
	if err := json.Unmarshal(preview.Body.Bytes(), &previewBody); err != nil {
		t.Fatal(err)
	}
	if previewBody.Applied || previewBody.Summary.FileCount != 1 || previewBody.DownloadedBytes != int64(bucket.liveBytes()) || previewBody.CompletedAt == "" {
		t.Errorf("preview measurements = %+v", previewBody)
	}
	if len(previewBody.Written) != 1 {
		t.Errorf("preview written = %v", previewBody.Written)
	}

	applyRequest, err := json.Marshal(map[string]any{
		"apply": true, "expectedETag": previewBody.RemoteETag,
		"expectedRevision": previewBody.RemoteRevision,
	})
	if err != nil {
		t.Fatal(err)
	}
	applied := sendSync(t, engine, http.MethodPost, "/api/v1/sync/pull", string(applyRequest))
	if applied.Code != http.StatusOK {
		t.Fatalf("apply = %d: %s", applied.Code, applied.Body.String())
	}
	var appliedBody struct {
		Applied         bool   `json:"applied"`
		DownloadedBytes int64  `json:"downloadedBytes"`
		CompletedAt     string `json:"completedAt"`
	}
	if err := json.Unmarshal(applied.Body.Bytes(), &appliedBody); err != nil {
		t.Fatal(err)
	}
	if !appliedBody.Applied || appliedBody.DownloadedBytes != int64(bucket.liveBytes()) || appliedBody.CompletedAt == "" {
		t.Errorf("apply measurements = %+v", appliedBody)
	}

	status := sendSync(t, engine, http.MethodGet, "/api/v1/sync", "")
	var statusBody struct {
		LastOperation *remotesync.SyncOperation `json:"lastOperation"`
	}
	if err := json.Unmarshal(status.Body.Bytes(), &statusBody); err != nil {
		t.Fatal(err)
	}
	if statusBody.LastOperation == nil || statusBody.LastOperation.Kind != remotesync.OperationApply ||
		statusBody.LastOperation.DownloadedBytes != int64(bucket.liveBytes()) || statusBody.LastOperation.Written != 1 {
		t.Errorf("applied operation = %+v", statusBody.LastOperation)
	}
}

func TestSyncConflictResponseIncludesPermissionChanges(t *testing.T) {
	for _, test := range []struct {
		name              string
		conflict          remotesync.Conflict
		changedHere       bool
		changedThere      bool
		remoteModePresent bool
	}{
		{
			name: "local permission and remote contents",
			conflict: remotesync.Conflict{
				Path: "keys/id_ed25519", BaseDigest: "same", LocalDigest: "same", RemoteDigest: "different",
				BaseMode: "0600", LocalMode: "0700", RemoteMode: "0600",
			},
			changedHere: true, changedThere: true, remoteModePresent: true,
		},
		{
			name: "remote permission only",
			conflict: remotesync.Conflict{
				Path: "script", BaseDigest: "same", LocalDigest: "same", RemoteDigest: "same",
				BaseMode: "0600", LocalMode: "0600", RemoteMode: "0700",
			},
			changedHere: false, changedThere: true, remoteModePresent: true,
		},
		{
			name: "remote deletion",
			conflict: remotesync.Conflict{
				Path: "deleted", BaseDigest: "same", LocalDigest: "same",
				BaseMode: "0600", LocalMode: "0600",
			},
			changedHere: false, changedThere: true, remoteModePresent: false,
		},
		{
			name: "created on both sides",
			conflict: remotesync.Conflict{
				Path: "new", LocalDigest: "local", RemoteDigest: "remote",
				LocalMode: "0600", RemoteMode: "0700",
			},
			changedHere: true, changedThere: true, remoteModePresent: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := syncConflictResponse(test.conflict)
			if response.ChangedHere != test.changedHere || response.ChangedThere != test.changedThere {
				t.Fatalf("changed flags = here %t, there %t", response.ChangedHere, response.ChangedThere)
			}
			if (response.RemoteMode != nil) != test.remoteModePresent {
				t.Fatalf("remote mode = %v, present want %t", response.RemoteMode, test.remoteModePresent)
			}
			if test.conflict.LocalMode != "" && (response.LocalMode == nil || *response.LocalMode != test.conflict.LocalMode) {
				t.Fatalf("local mode = %v, want %q", response.LocalMode, test.conflict.LocalMode)
			}
		})
	}
}

func TestPullCanPreviewAndApplyRemoteWinsForAConflictingWorkspace(t *testing.T) {
	bucket := &measuredSyncBucket{}
	_, producer, _ := measuredSyncEngine(t, bucket, map[string]string{"config": "Host remote\n"})
	if _, err := producer.Push(context.Background(), measuredSyncKey, "Remote workspace"); err != nil {
		t.Fatal(err)
	}
	engine, _, _ := measuredSyncEngine(t, bucket, map[string]string{"config": "Host local\n"})

	conflicted := sendSync(t, engine, http.MethodPost, "/api/v1/sync/pull", `{"apply":false}`)
	if conflicted.Code != http.StatusOK || !strings.Contains(conflicted.Body.String(), `"path":"config"`) {
		t.Fatalf("conflict preview = %d: %s", conflicted.Code, conflicted.Body.String())
	}

	preview := sendSync(t, engine, http.MethodPost, "/api/v1/sync/pull", `{"apply":false,"resolve":"remote"}`)
	if preview.Code != http.StatusOK {
		t.Fatalf("remote-wins preview = %d: %s", preview.Code, preview.Body.String())
	}
	var generation struct {
		Conflicts      []api.SyncConflict `json:"conflicts"`
		Written        []string           `json:"written"`
		RemoteETag     string             `json:"remoteETag"`
		RemoteRevision string             `json:"remoteRevision"`
	}
	if err := json.Unmarshal(preview.Body.Bytes(), &generation); err != nil {
		t.Fatal(err)
	}
	if len(generation.Conflicts) != 0 || !slices.Contains(generation.Written, "config") {
		t.Fatalf("remote-wins preview = %+v", generation)
	}

	request, err := json.Marshal(map[string]any{
		"apply": true, "resolve": "remote",
		"expectedETag": generation.RemoteETag, "expectedRevision": generation.RemoteRevision,
	})
	if err != nil {
		t.Fatal(err)
	}
	applied := sendSync(t, engine, http.MethodPost, "/api/v1/sync/pull", string(request))
	if applied.Code != http.StatusOK || !strings.Contains(applied.Body.String(), `"applied":true`) {
		t.Fatalf("remote-wins apply = %d: %s", applied.Code, applied.Body.String())
	}
}

func TestReceiveOnlyCanPreviewAndApplyAnExplicitRemoteHead(t *testing.T) {
	bucket := &measuredSyncBucket{}
	receiver := newMeasuredSyncInstallation(t, bucket, map[string]string{"config": "Host original\n"})
	if _, err := receiver.service.Push(context.Background(), measuredSyncKey, "Initial snapshot"); err != nil {
		t.Fatal(err)
	}

	_, replacement, _ := measuredSyncEngine(t, bucket, map[string]string{"config": "Host replacement\n"})
	if _, err := replacement.ForcePush(
		context.Background(), measuredSyncKey, bucket.liveETag(), "Replace head",
	); err != nil {
		t.Fatal(err)
	}

	receiveOnly := receiver.config
	receiveOnly.Direction = remotesync.DirectionPull
	if err := receiver.service.Configure(receiveOnly, receiver.credentials, receiver.client); err != nil {
		t.Fatal(err)
	}
	preview := sendSync(t, receiver.engine, http.MethodPost, "/api/v1/sync/pull",
		`{"apply":false,"resolve":"remote","acceptRemoteHead":true}`)
	if preview.Code != http.StatusOK {
		t.Fatalf("explicit remote preview = %d: %s", preview.Code, preview.Body.String())
	}
	var generation struct {
		RemoteETag     string `json:"remoteETag"`
		RemoteRevision string `json:"remoteRevision"`
	}
	if err := json.Unmarshal(preview.Body.Bytes(), &generation); err != nil {
		t.Fatal(err)
	}
	request, err := json.Marshal(map[string]any{
		"apply": true, "resolve": "remote", "acceptRemoteHead": true,
		"expectedETag": generation.RemoteETag, "expectedRevision": generation.RemoteRevision,
	})
	if err != nil {
		t.Fatal(err)
	}
	applied := sendSync(t, receiver.engine, http.MethodPost, "/api/v1/sync/pull", string(request))
	if applied.Code != http.StatusOK || !strings.Contains(applied.Body.String(), `"applied":true`) {
		t.Fatalf("explicit remote apply = %d: %s", applied.Code, applied.Body.String())
	}
	_, contents, err := receiver.service.Collect()
	if err != nil {
		t.Fatal(err)
	}
	if got := string(contents["config"]); got != "Host replacement\n" {
		t.Fatalf("received config = %q", got)
	}
}

func TestExplicitRemoteHeadIsAvailableToReceiversAndRefusedForSendOnly(t *testing.T) {
	bucket := &measuredSyncBucket{}
	installation := newMeasuredSyncInstallation(t, bucket, map[string]string{"config": "Host one\n"})
	if _, err := installation.service.Push(context.Background(), measuredSyncKey, "Initial snapshot"); err != nil {
		t.Fatal(err)
	}
	response := sendSync(t, installation.engine, http.MethodPost, "/api/v1/sync/pull",
		`{"apply":false,"resolve":"remote","acceptRemoteHead":true}`)
	if response.Code != http.StatusOK {
		t.Fatalf("explicit remote head in bidirectional mode = %d: %s", response.Code, response.Body.String())
	}
	sendOnly := installation.config
	sendOnly.Direction = remotesync.DirectionPush
	if err := installation.service.Configure(sendOnly, installation.credentials, installation.client); err != nil {
		t.Fatal(err)
	}
	response = sendSync(t, installation.engine, http.MethodPost, "/api/v1/sync/pull",
		`{"apply":false,"resolve":"remote","acceptRemoteHead":true}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("explicit remote head in send-only mode = %d, want 400: %s", response.Code, response.Body.String())
	}
}

func TestSyncProblemClassifiesLocalWorkspaceRaces(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code string
	}{
		{name: "changed", err: &storage.ConflictError{Path: "config"}, code: "sync_local_changed"},
		{name: "busy", err: storage.ErrWorkspaceBusy, code: "sync_workspace_busy"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine := echo.New()
			engine.GET("/problem", func(c *echo.Context) error { return syncProblem(c, test.err) })
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/problem", nil))
			if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), `"code":"`+test.code+`"`) {
				t.Fatalf("problem = %d: %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestApplyRejectsARemoteGenerationThatChangedAfterPreview(t *testing.T) {
	bucket := &measuredSyncBucket{}
	_, producer, _ := measuredSyncEngine(t, bucket, map[string]string{"config": "Host first\n"})
	if _, err := producer.Push(context.Background(), measuredSyncKey, "Initial snapshot"); err != nil {
		t.Fatal(err)
	}
	engine, _, _ := measuredSyncEngine(t, bucket, map[string]string{})
	preview := sendSync(t, engine, http.MethodPost, "/api/v1/sync/pull", `{"apply":false}`)
	if preview.Code != http.StatusOK {
		t.Fatalf("preview = %d: %s", preview.Code, preview.Body.String())
	}
	var generation struct {
		RemoteETag     string `json:"remoteETag"`
		RemoteRevision string `json:"remoteRevision"`
	}
	if err := json.Unmarshal(preview.Body.Bytes(), &generation); err != nil {
		t.Fatal(err)
	}

	_, replacement, _ := measuredSyncEngine(t, bucket, map[string]string{"config": "Host second\n"})
	if _, err := replacement.ForcePush(context.Background(), measuredSyncKey, bucket.liveETag(), "Replacement snapshot"); err != nil {
		t.Fatal(err)
	}
	request, err := json.Marshal(map[string]any{
		"apply": true, "expectedETag": generation.RemoteETag,
		"expectedRevision": generation.RemoteRevision,
	})
	if err != nil {
		t.Fatal(err)
	}
	applied := sendSync(t, engine, http.MethodPost, "/api/v1/sync/pull", string(request))
	if applied.Code != http.StatusConflict || !strings.Contains(applied.Body.String(), `"code":"preview_stale"`) {
		t.Fatalf("stale apply = %d: %s", applied.Code, applied.Body.String())
	}
}

func TestApplyWithoutPreviewGenerationIsRefused(t *testing.T) {
	bucket := &measuredSyncBucket{}
	_, producer, _ := measuredSyncEngine(t, bucket, map[string]string{"config": "Host first\n"})
	if _, err := producer.Push(context.Background(), measuredSyncKey, "Initial snapshot"); err != nil {
		t.Fatal(err)
	}
	engine, _, _ := measuredSyncEngine(t, bucket, map[string]string{})
	response := sendSync(t, engine, http.MethodPost, "/api/v1/sync/pull", `{"apply":true}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("apply without preview generation = %d, want 400: %s", response.Code, response.Body.String())
	}
}

func TestTheDirectionIsReportedAndDefaultsToBoth(t *testing.T) {
	engine, _ := syncEngine(t)

	recorder := send(t, engine, http.MethodGet, "/api/v1/sync", "", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/sync = %d: %s", recorder.Code, recorder.Body.String())
	}
	var status struct {
		Direction string `json:"direction"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	// 一度も設定されたことのないマシンは、一方向モードにあるのではなく
	// どのモードにもない。その安全な解釈は、普通のモードとみなすことである。
	if status.Direction != "both" {
		t.Errorf("direction = %q, want both", status.Direction)
	}
}

func TestSettingsCarryTheDirectionThroughToTheService(t *testing.T) {
	engine, service := syncEngine(t)

	for _, direction := range []string{"push", "pull", "both"} {
		recorder := send(t, engine, http.MethodPut, "/api/v1/sync/settings", settings(direction), nil)
		if recorder.Code != http.StatusOK {
			t.Fatalf("PUT with %q = %d: %s", direction, recorder.Code, recorder.Body.String())
		}
		if got := string(service.Direction()); got != direction {
			t.Errorf("after %q the service reports %q", direction, got)
		}
		var status struct {
			Direction string `json:"direction"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &status); err != nil {
			t.Fatal(err)
		}
		if status.Direction != direction {
			t.Errorf("the response reports %q after setting %q", status.Direction, direction)
		}
	}
}

func TestSettingsWithoutADirectionAreRefused(t *testing.T) {
	engine, service := syncEngine(t)
	if recorder := send(t, engine, http.MethodPut, "/api/v1/sync/settings", settings("pull"), nil); recorder.Code != http.StatusOK {
		t.Fatalf("PUT = %d", recorder.Code)
	}
	if recorder := send(t, engine, http.MethodPut, "/api/v1/sync/settings", settings(""), nil); recorder.Code != http.StatusBadRequest {
		t.Fatalf("PUT without direction = %d, want 400", recorder.Code)
	}
	if got := service.Direction(); got != remotesync.DirectionPull {
		t.Errorf("direction = %q after refused settings, want previous pull", got)
	}
}

func TestAnUnknownDirectionIsRefusedRatherThanIgnored(t *testing.T) {
	engine, service := syncEngine(t)
	if recorder := send(t, engine, http.MethodPut, "/api/v1/sync/settings", settings("pull"), nil); recorder.Code != http.StatusOK {
		t.Fatalf("PUT = %d", recorder.Code)
	}

	recorder := send(t, engine, http.MethodPut, "/api/v1/sync/settings", settings("sideways"), nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("PUT with an unknown direction = %d, want 400: %s", recorder.Code, recorder.Body.String())
	}
	// そして何も変わらなかった。すでに半分だけ適用してしまった拒否済み
	// リクエストは、全部適用してしまうより始末が悪い。
	if got := service.Direction(); got != remotesync.DirectionPull {
		t.Errorf("direction = %q after a refused request, want the previous pull", got)
	}
}

func TestARefusedDirectionIsAConflictAndNotAGatewayFailure(t *testing.T) {
	engine, _ := syncEngine(t)
	if recorder := send(t, engine, http.MethodPut, "/api/v1/sync/settings", settings("pull"), nil); recorder.Code != http.StatusOK {
		t.Fatalf("PUT = %d", recorder.Code)
	}

	recorder := send(t, engine, http.MethodPost, "/api/v1/sync/push", `{"message":"Test receive-only mode"}`, nil)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("POST /push on a receive-only machine = %d, want 409: %s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	// code は設定を指定しなければならない。"sync_failed" では、
	// このマシンが行った拒否なのに、ユーザーは自分の bucket を疑ってしまう。
	if body.Code != "sync_push_refused" {
		t.Errorf("code = %q, want sync_push_refused", body.Code)
	}
}

// 設定は保存されるため、2 回目の実行でもそれが残っている。決して
// 外へ漏れてはならないのは access key である。status は画面が読む
// ものであり、bucket の場所と vault がロック中かどうかだけを伝える。
func TestSyncStatusNeverCarriesTheAccessKey(t *testing.T) {
	engine, service, secrets := syncEngineWithVault(t)
	_ = service
	if err := secrets.Initialise(syncTestPassphrase); err != nil {
		t.Fatal(err)
	}

	configure := `{"endpoint":"https://s3.example.invalid","bucket":"b","region":"auto",` +
		`"accessKeyId":"AKIAEXAMPLE","secretAccessKey":"s3cret-key","direction":"both"}`
	if code := sendSync(t, engine, http.MethodPut, "/api/v1/sync/settings", configure).Code; code != http.StatusOK {
		t.Fatalf("configure = %d", code)
	}

	response := sendSync(t, engine, http.MethodGet, "/api/v1/sync", "")
	for _, absent := range []string{"AKIAEXAMPLE", "s3cret-key"} {
		if strings.Contains(response.Body.String(), absent) {
			t.Errorf("the status carries %q: %s", absent, response.Body.String())
		}
	}
	if !strings.Contains(response.Body.String(), "s3.example.invalid") {
		t.Errorf("the status does not say where the bucket is: %s", response.Body.String())
	}
}

// リージョンは秘密ではない。それは署名スコープに入る値であり、R2 の "auto" と
// 本物の AWS のバケットのリージョンとでは違うので、画面が現在値を出せなければ
// 利用者は自分が何を設定したのかを確かめられない。
func TestSyncStatusSaysWhichRegionIsConfigured(t *testing.T) {
	engine, _, secrets := syncEngineWithVault(t)
	if err := secrets.Initialise(syncTestPassphrase); err != nil {
		t.Fatal(err)
	}

	configure := `{"endpoint":"https://s3.example.invalid","bucket":"b","region":"eu-west-2",` +
		`"accessKeyId":"AKIAEXAMPLE","secretAccessKey":"s3cret-key","direction":"both"}`
	if code := sendSync(t, engine, http.MethodPut, "/api/v1/sync/settings", configure).Code; code != http.StatusOK {
		t.Fatalf("configure = %d", code)
	}

	response := sendSync(t, engine, http.MethodGet, "/api/v1/sync", "")
	if !strings.Contains(response.Body.String(), "eu-west-2") {
		t.Errorf("the status does not say which region: %s", response.Body.String())
	}
}

// 起動時には何も尋ねないため、画面は「設定済みなのに壊れている」
// ように見える空の form を出す代わりに、なぜ空なのかを言えなければならない。
func TestSyncStatusSaysWhenTheVaultIsShut(t *testing.T) {
	engine, _, secrets := syncEngineWithVault(t)
	if err := secrets.Initialise(syncTestPassphrase); err != nil {
		t.Fatal(err)
	}
	secrets.Lock()

	response := sendSync(t, engine, http.MethodGet, "/api/v1/sync", "")
	if !strings.Contains(response.Body.String(), `"locked":true`) {
		t.Errorf("the status does not say the vault is shut: %s", response.Body.String())
	}
}

func TestConfiguringRefusesAShutVault(t *testing.T) {
	engine, _, secrets := syncEngineWithVault(t)
	if err := secrets.Initialise(syncTestPassphrase); err != nil {
		t.Fatal(err)
	}
	secrets.Lock()

	configure := `{"endpoint":"https://s3.example.invalid","bucket":"b","accessKeyId":"k","secretAccessKey":"s","direction":"both"}`
	if code := sendSync(t, engine, http.MethodPut, "/api/v1/sync/settings", configure).Code; code != http.StatusConflict {
		t.Errorf("configure while locked = %d, want 409", code)
	}
}

// endpoint は打ち込まれたとおりではなく、使われる形で保存される。
//
// 末尾のスラッシュは、スナップショットの行き先を画面が示すところ
// どこでも "https://host//bucket" を生んでいた。リクエスト自体には
// それは含まれない。client がパス全体を置き換えるからだ。ので、
// これはユーザーに見せて自分の bucket と認識させる値についての話である。
func TestATrailingSlashOnTheEndpointIsRemoved(t *testing.T) {
	engine, service, secrets := syncEngineWithVault(t)
	if err := secrets.Initialise(syncTestPassphrase); err != nil {
		t.Fatal(err)
	}
	body := `{"endpoint":"https://s3.example.invalid/","bucket":"b","accessKeyId":"k","secretAccessKey":"s","direction":"both"}`
	if code := sendSync(t, engine, http.MethodPut, "/api/v1/sync/settings", body).Code; code != http.StatusOK {
		t.Fatalf("configure = %d", code)
	}
	endpoint, _, _, _ := service.Target()
	if endpoint != "https://s3.example.invalid" {
		t.Errorf("endpoint = %q, want the trailing slash gone", endpoint)
	}
}

// パス付きの endpoint は暗黙に切り詰めるのではなく拒否する。client
// はパスを /bucket/key に置き換えるため、貼り付けられた
// "…/my-bucket" は何も言わずに捨てられ、ユーザーはこの application が
// 一度も書いたことのない場所にオブジェクトを探すことになる。
func TestAnEndpointWithAPathIsRefused(t *testing.T) {
	engine, _, secrets := syncEngineWithVault(t)
	if err := secrets.Initialise(syncTestPassphrase); err != nil {
		t.Fatal(err)
	}
	body := `{"endpoint":"https://s3.example.invalid/my-bucket","bucket":"b","accessKeyId":"k","secretAccessKey":"s","direction":"both"}`
	recorder := sendSync(t, engine, http.MethodPut, "/api/v1/sync/settings", body)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("configure with a path = %d, want 400", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "endpoint_must_have_no_path") {
		t.Errorf("code = %s", recorder.Body.String())
	}
}

func TestSyncRuntimeValidationRejectsSchemaBypasses(t *testing.T) {
	engine, _ := syncEngine(t)
	for _, endpoint := range []string{
		"https://user:password@example.invalid",
		"https://example.invalid?bucket=other",
		"https://example.invalid#fragment",
		"https://example.invalid/%2f",
		"https:opaque-endpoint",
	} {
		body, err := json.Marshal(map[string]any{
			"endpoint": endpoint, "bucket": "sshc", "accessKeyId": "AKID",
			"secretAccessKey": "secret", "direction": "both",
		})
		if err != nil {
			t.Fatal(err)
		}
		if response := sendSync(t, engine, http.MethodPut, "/api/v1/sync/settings", string(body)); response.Code != http.StatusBadRequest {
			t.Errorf("endpoint %q = %d, want 400: %s", endpoint, response.Code, response.Body.String())
		}
	}
	tooLong := strings.Repeat("x", 513)
	settingsBody, _ := json.Marshal(map[string]any{
		"endpoint": "https://example.invalid", "bucket": "sshc", "accessKeyId": tooLong,
		"secretAccessKey": "secret", "direction": "both",
	})
	if response := sendSync(t, engine, http.MethodPut, "/api/v1/sync/settings", string(settingsBody)); response.Code != http.StatusBadRequest {
		t.Errorf("oversized access key = %d, want 400", response.Code)
	}
	keyBody, _ := json.Marshal(map[string]string{"key": strings.Repeat("k", 1025)})
	if response := sendSync(t, engine, http.MethodPut, "/api/v1/sync/key", string(keyBody)); response.Code != http.StatusBadRequest {
		t.Errorf("oversized sync key = %d, want 400", response.Code)
	}
	if response := sendSync(t, engine, http.MethodPost, "/api/v1/sync/pull",
		`{"apply":true,"expectedETag":"etag","expectedRevision":"NOT-A-REVISION"}`); response.Code != http.StatusBadRequest {
		t.Errorf("invalid revision = %d, want 400", response.Code)
	}
	historyBody, _ := json.Marshal(map[string]string{"key": strings.Repeat("h", 1025)})
	if response := sendSync(t, engine, http.MethodPost, "/api/v1/sync/history/diff", string(historyBody)); response.Code != http.StatusBadRequest {
		t.Errorf("oversized history key = %d, want 400", response.Code)
	}
}

func TestReplacingASyncKeyRequiresHistoryLossConfirmation(t *testing.T) {
	bucket := &measuredSyncBucket{}
	engine, service, _ := measuredSyncEngine(t, bucket, map[string]string{"config": "Host edge\n"})
	if _, err := service.Push(context.Background(), measuredSyncKey, "Initial snapshot"); err != nil {
		t.Fatal(err)
	}
	const next = "ZX98-YW76-VU54-TS32-RQ10-PO98"
	body, _ := json.Marshal(map[string]any{"key": next})
	refused := sendSync(t, engine, http.MethodPut, "/api/v1/sync/key", string(body))
	if refused.Code != http.StatusConflict ||
		!strings.Contains(refused.Body.String(), "sync_history_key_loss_confirmation_required") {
		t.Fatalf("unconfirmed replacement = %d: %s", refused.Code, refused.Body.String())
	}
	confirmedBody, _ := json.Marshal(map[string]any{"key": next, "confirmHistoryLoss": true})
	confirmed := sendSync(t, engine, http.MethodPut, "/api/v1/sync/key", string(confirmedBody))
	if confirmed.Code != http.StatusOK {
		t.Fatalf("confirmed replacement = %d: %s", confirmed.Code, confirmed.Body.String())
	}
}

// 設定は保持される前に試される。
//
// 応答しない bucket は、ユーザーが打ち間違えた bucket である。その
// 間違いを保存してしまうと、直せるはずのこの画面ではなく最初の
// push で失敗が表面化することになる。何も保存されず何も設定
// されない。中途半端に適用された拒否は、何もしないより始末が悪い。
func TestSettingsThatCannotReachTheBucketAreNotStored(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	manager := storage.NewManager(workspace, time.Now, rand.Reader)
	service := remotesync.NewService(workspace, manager,
		func() string { return "2026-08-05T00:00:00Z" },
		func() (string, error) { return "origin-test", nil })
	secrets := secret.NewService(workspace, manager, time.Now)
	if err := secrets.Initialise(syncTestPassphrase); err != nil {
		t.Fatal(err)
	}
	engine := echo.New()
	registerSyncRoutes(engine, SyncHandlers{
		Service: service, Secrets: secrets,
		Reach: func(context.Context, *objectstore.Client, string) error { return objectstore.ErrRefused },
	})

	body := `{"endpoint":"https://s3.example.invalid","bucket":"b","accessKeyId":"k","secretAccessKey":"s","direction":"both"}`
	recorder := sendSync(t, engine, http.MethodPut, "/api/v1/sync/settings", body)
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("configure against an unreachable bucket = %d, want 502: %s", recorder.Code, recorder.Body.String())
	}
	if service.Configured() {
		t.Error("the service was configured with settings that do not work")
	}
	if settings, err := secrets.SyncSettings(); err != nil || settings.Bucket != "" {
		t.Errorf("the settings were stored anyway: %+v (%v)", settings, err)
	}
}

func TestSyncConnectionFailuresHaveSpecificSafeCodes(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "timeout", err: context.DeadlineExceeded, status: http.StatusGatewayTimeout, code: "bucket_timeout"},
		{name: "dns", err: &net.DNSError{Err: "no such host", Name: "private.example"}, status: http.StatusBadGateway, code: "bucket_dns_failed"},
		{name: "network", err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("offline")}, status: http.StatusBadGateway, code: "bucket_unreachable"},
		{name: "internal", err: errors.New("unclassified failure"), status: http.StatusInternalServerError, code: "sync_internal_failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine, service, secrets := syncEngineWithVault(t)
			if err := secrets.Initialise(syncTestPassphrase); err != nil {
				t.Fatal(err)
			}
			engine = echo.New()
			registerSyncRoutes(engine, SyncHandlers{
				Service: service, Secrets: secrets,
				Reach: func(context.Context, *objectstore.Client, string) error { return test.err },
			})
			recorder := sendSync(t, engine, http.MethodPut, "/api/v1/sync/settings", settings("pull"))
			if recorder.Code != test.status || !strings.Contains(recorder.Body.String(), `"code":"`+test.code+`"`) {
				t.Fatalf("response = %d %s, want %d %s", recorder.Code, recorder.Body.String(), test.status, test.code)
			}
			if strings.Contains(recorder.Body.String(), "private.example") || strings.Contains(recorder.Body.String(), "unclassified failure") {
				t.Fatalf("unsafe underlying detail escaped: %s", recorder.Body.String())
			}
		})
	}
}

// 押したユーザーが打つものは、もう無い。暗号化する鍵は保管庫の中にあり、
// 保管庫が開いていることが同期してよいことの唯一の条件である。鍵をまだ
// 決めていないなら、押しても何も起きてはならない。リモートには、誰も
// 開けられない書庫が残るからだ。
func TestPushWithoutAKeyRefusesAndRunsNothing(t *testing.T) {
	engine, _, secrets := syncEngineWithVault(t)
	if err := secrets.Initialise(syncTestPassphrase); err != nil {
		t.Fatal(err)
	}
	if code := sendSync(t, engine, http.MethodPut, "/api/v1/sync/settings", settings("both")).Code; code != http.StatusOK {
		t.Fatalf("configure = %d", code)
	}

	recorder := sendSync(t, engine, http.MethodPost, "/api/v1/sync/push", `{"message":"Test missing key"}`)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("push without a key = %d, want 409: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "sync_key_missing") {
		t.Errorf("code = %s", recorder.Body.String())
	}
	// そしてそこで止まった。拒否を書き込んでからそれでも実行してしまう
	// ハンドラは、body に両方の結果を残してしまう。
	if strings.Contains(recorder.Body.String(), "sync_failed") {
		t.Errorf("the push ran after being refused: %s", recorder.Body.String())
	}
}

// vault を一度も作ったことのないマシンは、初めての pull を行う
// マシンである。打ち込まれるパスワードはアーカイブへの鍵であり、
// ここでは確認できない。確認できるのはアーカイブ自身だけだ。
func TestPullOnAMachineWithNoVaultIsNotRefusedForTheWrongReason(t *testing.T) {
	engine, _ := syncEngine(t)
	if code := sendSync(t, engine, http.MethodPut, "/api/v1/sync/settings", settings("both")).Code; code != http.StatusOK {
		t.Fatalf("configure = %d", code)
	}

	recorder := sendSync(t, engine, http.MethodPost, "/api/v1/sync/pull", `{"passphrase":"a password for a vault that is not here"}`)
	if recorder.Code == http.StatusForbidden && strings.Contains(recorder.Body.String(), "wrong_master_password") {
		t.Errorf("a machine with no vault was told its master password was wrong: %s", recorder.Body.String())
	}
}

// path は保存され、応答にも返される。そして bucket 名と同じくらい
// 狭く絞られている。どちらもこの application が署名する URL のセグメントになるからだ。
func TestTheObjectPathIsStoredAndRefusedWhenItCouldEscape(t *testing.T) {
	engine, service, secrets := syncEngineWithVault(t)
	if err := secrets.Initialise(syncTestPassphrase); err != nil {
		t.Fatal(err)
	}

	body := `{"endpoint":"https://s3.example.invalid","bucket":"b","path":"/laptops/","accessKeyId":"k","secretAccessKey":"s","direction":"both"}`
	recorder := sendSync(t, engine, http.MethodPut, "/api/v1/sync/settings", body)
	if recorder.Code != http.StatusOK {
		t.Fatalf("configure = %d: %s", recorder.Code, recorder.Body.String())
	}
	if _, _, path, _ := service.Target(); path != "laptops" {
		t.Errorf("path = %q, want it trimmed to laptops", path)
	}
	if !strings.Contains(recorder.Body.String(), `"path":"laptops"`) {
		t.Errorf("the status does not report the path: %s", recorder.Body.String())
	}

	for _, unsafe := range []string{"../elsewhere", "a//b", "a b"} {
		escaping := `{"endpoint":"https://s3.example.invalid","bucket":"b","path":"` + unsafe +
			`","accessKeyId":"k","secretAccessKey":"s","direction":"both"}`
		if code := sendSync(t, engine, http.MethodPut, "/api/v1/sync/settings", escaping).Code; code != http.StatusBadRequest {
			t.Errorf("configure with path %q = %d, want 400", unsafe, code)
		}
	}
	// そして拒否によって何も変わらなかった。
	if _, _, path, _ := service.Target(); path != "laptops" {
		t.Errorf("path = %q after refusals, want laptops", path)
	}
}

// 切ったことは保管庫の中に残る。この実行のあいだだけ止まる切り方は、次に
// 起動したときに暗黙に再開することであり、止めたユーザーはそれを止めたと思っている。
func TestTheAutoSyncSwitchIsRememberedAndReported(t *testing.T) {
	engine, _, secrets := syncEngineWithVault(t)
	if err := secrets.Initialise(syncTestPassphrase); err != nil {
		t.Fatal(err)
	}

	recorder := sendSync(t, engine, http.MethodPut, "/api/v1/sync/auto", `{"enabled":true}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("PUT /auto = %d: %s", recorder.Code, recorder.Body.String())
	}
	settings, err := secrets.SyncSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !settings.Auto {
		t.Fatal("the switch was not written to the vault")
	}

	// そして status がそれを言う。
	var body struct {
		Auto struct {
			Enabled bool   `json:"enabled"`
			Phase   string `json:"phase"`
		} `json:"auto"`
	}
	status := sendSync(t, engine, http.MethodGet, "/api/v1/sync", "")
	if err := json.Unmarshal(status.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	// この engine には巡回が繋がっていないので、入っていても走る場所が無い。
	// 画面が読むのは phase であり、そこは常に埋まっていなければならない。
	if body.Auto.Phase == "" {
		t.Fatalf("status carried no phase: %s", status.Body.String())
	}

	if code := sendSync(t, engine, http.MethodPut, "/api/v1/sync/auto", `{"enabled":false}`).Code; code != http.StatusOK {
		t.Fatalf("PUT /auto off = %d", code)
	}
	settings, err = secrets.SyncSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.Auto {
		t.Fatal("the switch stayed on after being turned off")
	}
}

func TestFreshAutoSyncIsReportedAsIdle(t *testing.T) {
	_, service, secrets := syncEngineWithVault(t)
	auto := remotesync.NewAuto(service, time.Minute, func() string { return "2026-08-24T00:00:00Z" })
	engine := echo.New()
	registerSyncRoutes(engine, SyncHandlers{Service: service, Secrets: secrets, Auto: auto, Reach: reachable})

	status := sendSync(t, engine, http.MethodGet, "/api/v1/sync", "")
	if status.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/sync = %d: %s", status.Code, status.Body.String())
	}
	var body struct {
		Auto struct {
			Enabled bool   `json:"enabled"`
			Phase   string `json:"phase"`
		} `json:"auto"`
	}
	if err := json.Unmarshal(status.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Auto.Enabled || body.Auto.Phase != "idle" {
		t.Errorf("fresh auto status = %+v, want disabled idle", body.Auto)
	}
}

// 巡回の無い設置で「今すぐ」を押しても、何も起きない。押せてしまえば、
// 画面は起きていないことを起きたと言うことになる。
func TestSyncNowWithoutALoopIsRefused(t *testing.T) {
	engine, _, secrets := syncEngineWithVault(t)
	if err := secrets.Initialise(syncTestPassphrase); err != nil {
		t.Fatal(err)
	}
	recorder := sendSync(t, engine, http.MethodPost, "/api/v1/sync/now", "")
	if recorder.Code != http.StatusConflict {
		t.Fatalf("POST /now = %d, want 409: %s", recorder.Code, recorder.Body.String())
	}
}

func TestSyncNowInternalFailureIsNotReportedAsHTTP200(t *testing.T) {
	engine := echo.New()
	engine.POST("/", func(c *echo.Context) error {
		return autoSyncFailureProblem(c, "sync_internal_failed")
	})
	recorder := sendSync(t, engine, http.MethodPost, "/", "")
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("internal auto failure = %d, want 500: %s", recorder.Code, recorder.Body.String())
	}
	var body api.Problem
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "sync_internal_failed" {
		t.Fatalf("problem = %+v", body)
	}
}

func TestSyncNowPropagatesAnAutomaticCycleFailure(t *testing.T) {
	_, service, secrets := syncEngineWithVault(t)
	if err := secrets.Initialise(syncTestPassphrase); err != nil {
		t.Fatal(err)
	}
	auto := remotesync.NewAuto(service, time.Minute, func() string { return "2026-08-31T00:00:00Z" })
	auto.Key = func() (string, bool) { return measuredSyncKey, true }
	engine := echo.New()
	registerSyncRoutes(engine, SyncHandlers{Service: service, Secrets: secrets, Auto: auto, Reach: reachable})
	recorder := sendSync(t, engine, http.MethodPost, "/api/v1/sync/now", "")
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "sync_not_configured") {
		t.Fatalf("POST /now failed cycle = %d: %s", recorder.Code, recorder.Body.String())
	}
}
