package httpserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"sshc/internal/application"
	sshcSFTP "sshc/internal/sftp"
)

func TestDownloadOffsetAcceptsOnlySingleOpenEndedRange(t *testing.T) {
	tests := []struct {
		header string
		size   int64
		want   int64
		ranged bool
		valid  bool
	}{
		{header: "", size: 10, want: 0, valid: true},
		{header: "bytes=4-", size: 10, want: 4, ranged: true, valid: true},
		{header: "bytes=10-", size: 10},
		{header: "bytes=-4", size: 10},
		{header: "bytes=1-2", size: 10},
		{header: "bytes=1-,4-", size: 10},
	}
	for _, test := range tests {
		offset, ranged, err := downloadOffset(test.header, test.size)
		if (err == nil) != test.valid || offset != test.want || ranged != test.ranged {
			t.Errorf("downloadOffset(%q, %d) = %d, %v, %v", test.header, test.size, offset, ranged, err)
		}
	}
}

func TestTransferErrorsUseFileTransferProblemCodes(t *testing.T) {
	tests := []struct {
		err    error
		status int
		code   string
	}{
		{err: sshcSFTP.ErrTransferTooLarge, status: http.StatusRequestEntityTooLarge, code: "sftp_transfer_too_large"},
		// Unlike sftp_transfer_limit, a spool that cannot be used does not clear by
		// waiting, so it must not reach the browser as the retried limit code.
		{err: fmt.Errorf("%w: %w", sshcSFTP.ErrSpoolUnavailable, os.ErrPermission), status: http.StatusServiceUnavailable, code: "sftp_spool_unavailable"},
		{err: fmt.Errorf("%w: %w", sshcSFTP.ErrSpoolFull, &os.PathError{Op: "write", Path: "download.part", Err: os.ErrInvalid}), status: http.StatusInsufficientStorage, code: "sftp_spool_full"},
	}
	for _, test := range tests {
		response := serveSFTPProblem(test.err)
		if response.Code != test.status ||
			!bytes.Contains(response.Body.Bytes(), []byte(`"code":"`+test.code+`"`)) {
			t.Errorf("%v = %d: %s, want %d %s", test.err, response.Code, response.Body.String(), test.status, test.code)
		}
	}
}

func TestTheEnginesOwnRefusalIsToldApartFromTheServers(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code string
	}{
		{name: "server", err: &os.PathError{Op: "open", Path: "/srv/report.txt", Err: os.ErrPermission}, code: "sftp_permission_denied"},
		{name: "local file permissions", err: fmt.Errorf("%w: %w", sshcSFTP.ErrLocalPermissionDenied, os.ErrPermission), code: "sftp_local_permission_denied"},
		{name: "macOS privacy protection", err: fmt.Errorf("%w: %w", sshcSFTP.ErrLocalPrivacyProtection, os.ErrPermission), code: "sftp_local_privacy_protection"},
	}
	for _, test := range tests {
		response := serveSFTPProblem(test.err)
		if response.Code != http.StatusForbidden || !bytes.Contains(response.Body.Bytes(), []byte(`"code":"`+test.code+`"`)) {
			t.Errorf("%s = %d: %s, want 403 %s", test.name, response.Code, response.Body.String(), test.code)
		}
	}
}

// serveSFTPProblem answers one request with the problem sftpProblem makes of err.
func serveSFTPProblem(err error) *httptest.ResponseRecorder {
	engine := echo.New()
	engine.GET("/sftp", func(c *echo.Context) error {
		return sftpProblem(c, err)
	})
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/sftp", nil))
	return response
}

func TestAnUnusableDownloadSpoolIsLoggedWhenTheTransferManagerStarts(t *testing.T) {
	notADirectory := filepath.Join(t.TempDir(), "sftp-spool")
	if err := os.WriteFile(notADirectory, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	transfers, err := newTransferManager(Options{
		SFTP:                  &sshcSFTP.Service{},
		SFTPDownloadSpoolRoot: notADirectory,
		Logger:                slog.New(slog.NewTextHandler(&logs, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer transfers.Close()
	// Only the base name is matched: the handler may quote a Windows path.
	if !strings.Contains(logs.String(), "SFTP downloads are unavailable") || !strings.Contains(logs.String(), "sftp-spool") {
		t.Fatalf("start log = %q, want the unusable spool and its cause", logs.String())
	}
}

func TestTheTransferManagerStartsWithTheStoredSettingsAndLogsTheOutOfRangeOnes(t *testing.T) {
	harness := newConfigHarness(t)
	// A hand-edited metadata.json can hold a value out of range.
	if _, err := harness.service.SetFileTransferSettings(application.FileTransferSettings{
		MaxConcurrent: 3, ProcessingStopped: true, LargeFileParallelism: sshcSFTP.MaxLargeFileParallelism + 1,
	}); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	transfers, err := newTransferManager(Options{
		Config:                harness.service,
		SFTPDownloadSpoolRoot: t.TempDir(),
		Logger:                slog.New(slog.NewTextHandler(&logs, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer transfers.Close()
	if transfers.MaxConcurrent() != 3 || !transfers.ProcessingStopped() ||
		transfers.LargeFileParallelism() != sshcSFTP.DefaultLargeFileParallelism {
		t.Fatalf("started with %d concurrent, stopped %v, %d connections",
			transfers.MaxConcurrent(), transfers.ProcessingStopped(), transfers.LargeFileParallelism())
	}
	if !strings.Contains(logs.String(), "largeFileParallelism") {
		t.Fatalf("start log = %q, want the setting replaced with its default", logs.String())
	}
}

// putTransferSettings sends what the transfer panel and `sshc sftp settings`
// send to the engine that newTransferManager starts over the harness's
// metadata.json.
func putTransferSettings(t *testing.T, harness *testHarness, body string) (*sshcSFTP.TransferManager, *httptest.ResponseRecorder) {
	t.Helper()
	manager, err := newTransferManager(Options{Config: harness.service, SFTPDownloadSpoolRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	engine := echo.New()
	registerSFTPRoutes(engine, SFTPHandlers{Transfers: manager})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/v1/sftp/transfers/settings", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(response, request)
	return manager, response
}

func TestSavedTransferSettingsAreAppliedAndKeptForTheNextStart(t *testing.T) {
	harness := newConfigHarness(t)
	manager, response := putTransferSettings(t, harness,
		`{"maxConcurrent":5,"clearCompletedAfterSeconds":600,"processingStopped":true,"largeFileThresholdBytes":104857600,"largeFileParallelism":2,"largeFileChunkBytes":33554432}`,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("save = %d: %s", response.Code, response.Body.String())
	}
	want := application.FileTransferSettings{
		MaxConcurrent: 5, ClearCompletedAfterSeconds: 600, ProcessingStopped: true,
		LargeFileThresholdBytes: 104857600, LargeFileParallelism: 2, LargeFileChunkBytes: 33554432,
	}
	if stored := harness.service.FileTransferSettings(); stored != want {
		t.Fatalf("metadata.json holds %+v, want %+v", stored, want)
	}
	if manager.MaxConcurrent() != 5 || manager.ClearCompletedAfter() != 10*time.Minute || !manager.ProcessingStopped() {
		t.Fatalf("the engine runs %d concurrent, clears after %v, stopped %v",
			manager.MaxConcurrent(), manager.ClearCompletedAfter(), manager.ProcessingStopped())
	}
}

func TestTransferSettingsThatCannotBeSavedAreNotApplied(t *testing.T) {
	harness := newConfigHarness(t)
	// A folder in place of metadata.json makes every save fail.
	if err := os.MkdirAll(filepath.Join(harness.workspace.StateDir(), application.MetadataFileName), 0o700); err != nil {
		t.Fatal(err)
	}
	manager, response := putTransferSettings(t, harness,
		`{"maxConcurrent":5,"clearCompletedAfterSeconds":0,"processingStopped":true,"largeFileThresholdBytes":104857600,"largeFileParallelism":1,"largeFileChunkBytes":33554432}`,
	)
	if response.Code < http.StatusBadRequest {
		t.Fatalf("save to an unwritable metadata.json = %d: %s", response.Code, response.Body.String())
	}
	if manager.MaxConcurrent() != sshcSFTP.DefaultTransferConcurrency || manager.ProcessingStopped() {
		t.Fatalf("the engine took settings that were not saved: %d concurrent, stopped %v",
			manager.MaxConcurrent(), manager.ProcessingStopped())
	}
}

func TestQueuedRemoteDeleteRequiresActionToken(t *testing.T) {
	manager := sshcSFTP.NewTransferManager(nil, t.TempDir())
	engine := echo.New()
	registerSFTPRoutes(engine, SFTPHandlers{Transfers: manager})
	body := []byte(`{"id":"delete_http_01","batchId":"delete_batch_01","batchName":"old","batchKind":"folder","alias":"edge","sourceAlias":"edge","sourcePath":"/old","operation":"delete","overwrite":false,"direction":"remote","kind":"folder","name":"old","remotePath":"/old","totalBytes":-1,"lastModified":0}`)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/sftp/transfers", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || !bytes.Contains(response.Body.Bytes(), []byte(`"code":"action_token_required"`)) {
		t.Fatalf("delete without action token = %d: %s", response.Code, response.Body.String())
	}
	jobs, err := manager.ListJobs()
	if err != nil || len(jobs) != 0 {
		t.Fatalf("delete without token was queued: %+v, %v", jobs, err)
	}
}

func TestTransferManagerHTTPContractAndSharedLimit(t *testing.T) {
	manager := sshcSFTP.NewTransferManager(nil, t.TempDir())
	manager.ConfigureJobs(1, nil)
	engine := echo.New()
	registerSFTPRoutes(engine, SFTPHandlers{Transfers: manager})

	create := func(id string) map[string]any {
		body, err := json.Marshal(map[string]any{
			"id": id, "batchId": "batch_http001", "batchName": "HTTP batch", "batchKind": "file",
			"alias": "edge", "direction": "upload", "kind": "file", "name": id + ".bin",
			"remotePath": "/" + id + ".bin", "totalBytes": 10, "lastModified": 123,
		})
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/sftp/transfers", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(response, request)
		if response.Code != http.StatusCreated {
			t.Fatalf("create %s = %d: %s", id, response.Code, response.Body.String())
		}
		var decoded map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
			t.Fatal(err)
		}
		return decoded
	}
	create("transfer_http01")
	create("transfer_http02")

	action := func(id, action string, transferred *int64) *httptest.ResponseRecorder {
		payload := map[string]any{"action": action}
		if transferred != nil {
			payload["transferredBytes"] = *transferred
		}
		body, _ := json.Marshal(payload)
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/sftp/transfers/"+id+"/actions", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(response, request)
		return response
	}
	if response := action("transfer_http01", "start", nil); response.Code != http.StatusOK {
		t.Fatalf("start = %d: %s", response.Code, response.Body.String())
	}
	if response := action("transfer_http02", "start", nil); response.Code != http.StatusConflict || !bytes.Contains(response.Body.Bytes(), []byte("sftp_transfer_limit")) {
		t.Fatalf("limit = %d: %s", response.Code, response.Body.String())
	}
	settings := httptest.NewRecorder()
	settingsRequest := httptest.NewRequest(http.MethodPut, "/api/v1/sftp/transfers/settings", bytes.NewBufferString(
		`{"maxConcurrent":3,"clearCompletedAfterSeconds":300,"processingStopped":false,"largeFileThresholdBytes":52428800,"largeFileParallelism":128,"largeFileChunkBytes":536870912}`,
	))
	settingsRequest.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(settings, settingsRequest)
	if settings.Code != http.StatusOK {
		t.Fatalf("update settings = %d: %s", settings.Code, settings.Body.String())
	}
	missingFingerprint := httptest.NewRecorder()
	engine.ServeHTTP(missingFingerprint, httptest.NewRequest(http.MethodPost,
		"/api/v1/sftp/edge/uploads/transfer_http01", bytes.NewBufferString(`{"path":"/transfer_http01.bin","size":10}`)))
	if missingFingerprint.Code != http.StatusBadRequest {
		t.Fatalf("upload without source fingerprint = %d: %s", missingFingerprint.Code, missingFingerprint.Body.String())
	}
	progress := int64(4)
	if response := action("transfer_http01", "progress", &progress); response.Code != http.StatusConflict {
		t.Fatalf("client-authored upload progress = %d: %s", response.Code, response.Body.String())
	}

	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/sftp/transfers", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("list = %d: %s", response.Code, response.Body.String())
	}
	var listed struct {
		MaxConcurrent           int   `json:"maxConcurrent"`
		LargeFileThresholdBytes int64 `json:"largeFileThresholdBytes"`
		LargeFileParallelism    int   `json:"largeFileParallelism"`
		LargeFileChunkBytes     int64 `json:"largeFileChunkBytes"`
		Jobs                    []struct {
			ID               string `json:"id"`
			BatchName        string `json:"batchName"`
			Status           string `json:"status"`
			TransferredBytes int64  `json:"transferredBytes"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if listed.MaxConcurrent != 3 || listed.LargeFileThresholdBytes != 50<<20 || listed.LargeFileParallelism != 128 || listed.LargeFileChunkBytes != 512<<20 ||
		len(listed.Jobs) != 2 || listed.Jobs[0].BatchName != "HTTP batch" || listed.Jobs[0].Status != "running" || listed.Jobs[0].TransferredBytes != 0 {
		t.Fatalf("listed = %+v", listed)
	}
	if _, err := manager.UpdateJob("transfer_http01", sshcSFTP.UpdateTransferJob{Action: sshcSFTP.TransferCancelAction}); err != nil {
		t.Fatalf("cancel transfer before clear: %v", err)
	}
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/v1/sftp/transfers/finished", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("clear finished = %d: %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/sftp/transfers", nil))
	var remaining struct {
		Jobs []struct {
			ID string `json:"id"`
		} `json:"jobs"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &remaining) != nil || len(remaining.Jobs) != 1 || remaining.Jobs[0].ID != "transfer_http02" {
		t.Fatalf("remaining after clear = %d: %s", response.Code, response.Body.String())
	}
	if _, err := manager.UpdateJob("transfer_http02", sshcSFTP.UpdateTransferJob{Action: sshcSFTP.TransferCancelAction}); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/v1/sftp/transfers/transfer_http02", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("remove one transfer = %d: %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/v1/sftp/transfers/transfer_http02", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("repeat remove one transfer = %d: %s", response.Code, response.Body.String())
	}
}
